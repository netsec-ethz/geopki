import click
import pandas as pd
import numpy as np
from tqdm import tqdm
from typing import Dict, List, Tuple, Optional, Set
from shapely import Polygon, MultiPolygon, box, to_geojson
from collections import deque
import matplotlib.pyplot as plt
import json
import os
import math
import gc
import hashlib
import sys

sys.path.insert(1, os.path.join(sys.path[0], '../../..'))  # noqa - prevent auto formatting
from coordinates import ZOrderBitString, GeodeticCoordinate

INITIAL_AREA_FRACTION = 1

# nodes
GEO_NODE_CSV_HEADER = "bit_string:ID(GeoNode),neighbor_hash,left_child_hash,right_child_hash\n"
CERTIFICATE_CSV_HEADER = "certificate_hash:ID(Certificate)\n"

# relationships
LEFT_CHILD_CSV_HEADER = ":START_ID(GeoNode),:END_ID(GeoNode)\n"
RIGHT_CHILD_CSV_HEADER = ":START_ID(GeoNode),:END_ID(GeoNode)\n"
INCLUDES_CSV_HEADER = ":START_ID(GeoNode),:END_ID(Certificate)\n"

DEFAULT_HASH = hashlib.sha256(b"\x00").digest()


class GeoCertificate:
    def __init__(
        self,
        domain: str,
        area: MultiPolygon,
        certificate_id: str,
        parents: np.ndarray,
        children: np.ndarray,
    ) -> None:

        self.domain = domain
        self.area = area
        self.certificate_id = certificate_id
        self.parents: List[str] = parents.tolist()
        self.children: List[str] = children.tolist()

    def to_cert(self) -> bytes:
        return json.dumps(
            {
                'domain': self.domain,
                'area': json.loads(to_geojson(self.area)),
                # 'certificate_id': self.certificate_id,
                # 'parents': self.parents,
                # 'children': self.children,
            },
            sort_keys=True
        ).encode("ascii")

    def hash(self) -> bytes:
        return hashlib.sha256(self.to_cert()).digest()


class BitStringRow:
    def __init__(
            self,
            bit_string: str,
            certificate_hashes: Set[bytes],
    ) -> None:

        self.bit_string = bit_string
        self.certificate_hashes = certificate_hashes

        self.left_child_hash: Optional[bytes] = None
        self.right_child_hash: Optional[bytes] = None
        self.neighbor_hash: Optional[bytes] = None
        self.hash = None

    def get_certificate_hashes(self) -> bytes:
        return b"".join(sorted(self.certificate_hashes))


def voxel_bounds_to_2d_wkt_polygon(bounds: Tuple[GeodeticCoordinate, GeodeticCoordinate]) -> str:
    voxel_min, voxel_max = bounds

    # points are in ccw order, i.e. (x_min, y_min), (x_max, y_min), (x_max, y_max), (x_min, y_max), (x_min, y_min)
    # longitude (x) first, latitude second, i.e. 'lon lat, lon lat, lon lat, ...'

    # SPECIAL CASE: NO EDGE CAN BE LONGER THAN 180 DEGREES OTHERWISE THE SHORTEST DISTANCE
    # IS IT'S COMPLEMENT (ANTIPODAL POINTS) -> ADD ANOTHER POINT IN BETWEEN
    if (
        voxel_max.longitude - voxel_min.longitude >= 180 or
        voxel_max.latitude - voxel_min.latitude >= 180
    ):
        poly = f"POLYGON((" +\
            f"{voxel_min.longitude} {voxel_min.latitude}" + f"," +\
            (
                f"{voxel_min.longitude + (voxel_max.longitude - voxel_min.longitude) / 2} {voxel_min.latitude},"
                if (voxel_max.longitude - voxel_min.longitude >= 180)
                else f""
            ) +\
            f"{voxel_max.longitude} {voxel_min.latitude}" + f"," +\
            (
                f"{voxel_max.longitude} {voxel_min.latitude + (voxel_max.latitude - voxel_min.latitude) / 2},"
                if (voxel_max.latitude - voxel_min.latitude >= 180)
                else f""
            ) +\
            f"{voxel_max.longitude} {voxel_max.latitude}" + f"," +\
            (
                f"{voxel_min.longitude + (voxel_max.longitude - voxel_min.longitude) / 2} {voxel_max.latitude},"
                if (voxel_max.longitude - voxel_min.longitude >= 180)
                else f""
            ) +\
            f"{voxel_min.longitude} {voxel_max.latitude}" + f"," +\
            (
                f"{voxel_min.longitude} {voxel_min.latitude + (voxel_max.latitude - voxel_min.latitude) / 2},"
                if (voxel_max.latitude - voxel_min.latitude >= 180)
                else f""
            ) +\
            f"{voxel_min.longitude} {voxel_min.latitude}" +\
            f"))"

        return f"'SRID=4326;{poly}'::geometry"
    else:
        return f"ST_SetSRID(ST_MakeBox2D(ST_Point({voxel_min.longitude}, {voxel_min.latitude}),ST_Point({voxel_max.longitude}, {voxel_max.latitude})),4326)"


def grow_initial_area(initial_area: ZOrderBitString, area: float, plot=False):
    # while the area of the object is larger than a fraction of the grid's area
    # decrease the grid's size
    while (
        initial_area.x_precision > 1 and
        ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        ).to_shapely_area().area * INITIAL_AREA_FRACTION < area
    ):
        if plot:
            x, y = initial_area.to_shapely_area().exterior.xy
            plt.plot(x, y, color="orange")

        # grow area by removing one bit
        initial_area = ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        )

    return initial_area


@click.command()
# the domain-location file
@click.argument('input_path', type=click.Path(exists=True))
# The place to store the output
@click.argument('output_path', type=click.Path(exists=False))
@click.option('--plot', 'plot', flag_value=True, default=False)
def main(
    input_path: str,
    output_path: str,
    plot: bool
):

    if input_path.endswith(".parquet"):
        df = pd.read_parquet(input_path)
    elif input_path.endswith(".csv"):
        df = pd.read_csv(input_path)
    else:
        raise Exception(f"Unkown input file type")

    if not os.path.isdir(output_path):
        raise Exception(f"Area output path has to be a directory")

    geo_certificates: List[GeoCertificate] = []
    bit_string_map: Dict[str, BitStringRow] = {}

    for row_i, (idx, row) in tqdm(
        enumerate(df.iterrows()),
        total=len(df),
        desc="locate buildings"
    ):

        if row['domain'] is None or not isinstance(row['domain'], str):
            continue

        # china
        # if row['certificate_id'] != "rel:270056":
        #     continue

        # ethz
        # if not "way:192151232" in row['certificate_id']:
        #     continue

        # 'certificate_id', 'list_of_multipolygons', 'parents', 'children', 'domain'
        valid_polygons = [
            polygon
            for polygon in row['list_of_multipolygons']
            # exclude not proper areas
            if polygon[0] == polygon[-1]
        ]

        if len(valid_polygons) <= 0:
            continue

        certificate_id = row['certificate_id']

        # China
        # if certificate_id == "rel:270056":
        #     plot = True

        parents = row['parents']
        children = row['children']
        domain = row['domain']

        multi_polygon = MultiPolygon(
            [
                Polygon(
                    [(x['lon'], x['lat']) for x in poly]
                )
                for poly in valid_polygons
            ]
        )

        geo_cert = GeoCertificate(
            domain=domain,
            area=multi_polygon,
            certificate_id=certificate_id,
            parents=parents,
            children=children,
        )

        geo_certificates.append(
            geo_cert
        )

        # for each polygon perform the search separately
        initial_areas = [
            (
                ZOrderBitString.from_bit_string(
                    ZOrderBitString.from_coordinate(
                        GeodeticCoordinate(
                            longitude=polygon[0]['lon'],
                            latitude=polygon[0]['lat'],
                            altitude=0
                        )
                    ).to_bit_string()
                    # remove z-bits
                    [:(ZOrderBitString.X_BITS + ZOrderBitString.Y_BITS)]
                ),
                Polygon(
                    [(x['lon'], x['lat']) for x in polygon]
                )
            )
            for polygon in valid_polygons
        ]

        intersecting_areas: List[str] = []

        # iterate over initial areas with they associated polygon
        # for each compute the intersecting voxels
        for initial_area, polygon in initial_areas:
            # walk around that voxel and find other intersecting ones
            # BFS
            visited: Dict[str, bool] = {}
            q = deque()
            q.append(
                grow_initial_area(
                    initial_area=initial_area,
                    # area=multi_polygon.area,
                    area=polygon.area,
                    plot=plot
                )
            )

            # print(initial_area.to_bit_string())
            if plot:
                x, y = polygon.exterior.xy
                plt.plot(x, y, color="b")

            while len(q) > 0:
                a: ZOrderBitString = q.popleft()
                bit_string = a.to_bit_string()

                if bit_string in visited:
                    continue

                # mark as visited
                visited[bit_string] = True

                asa = a.to_shapely_area()

                # check for intersection
                if not (
                    asa.intersects(polygon)
                ):
                    # if plot:
                    #     x, y = asa.exterior.xy
                    #     plt.plot(x, y, color="r")

                    continue

                # if plot:
                #     x, y = asa.exterior.xy
                #     plt.plot(x, y, color="purple", linewidth=5)

                # add to intersection list
                intersecting_areas.append(bit_string)

                # visit neighbors of a
                for dx in [-1, 0, 1]:
                    for dy in [-1, 0, 1]:

                        y_next = (
                            a.y_min + dy *
                            (1 << (ZOrderBitString.Y_BITS - a.y_precision))
                        )

                        x_next = (
                            a.x_min + dx *
                            (1 << (ZOrderBitString.X_BITS - a.x_precision))
                        ) % ZOrderBitString.C_X

                        if y_next < 0:
                            # the y-coordinate 'flips', we can account for this
                            # by only rotating around x and set y to 0
                            y_next = 0
                            # if we overflow, the x coordinate wraps around
                            x_next = (
                                x_next + math.floor(ZOrderBitString.C_X / 2)
                            ) % ZOrderBitString.C_X
                        elif y_next >= ZOrderBitString.C_Y:
                            # the y-coordinate 'flips', we can account for this
                            # by rotating around x and set y to C_Y - step size = original y
                            y_next = a.y_min
                            # if we overflow the x coordinate wraps around
                            x_next = (
                                x_next + math.floor(ZOrderBitString.C_X / 2)
                            ) % ZOrderBitString.C_X

                        # clear bottom bits of the x coordinate, might be messed up after wrapping around
                        bl = len(bin(x_next)[2:]) - a.x_precision
                        if bl > 0:
                            x_next = (x_next >> bl) << bl

                        q.append(
                            ZOrderBitString(
                                x_min=x_next,
                                x_precision=a.x_precision,
                                y_min=y_next,
                                y_precision=a.y_precision,
                                # z will stay the same
                                z_min=a.z_min,
                                z_precision=a.z_precision,
                            )
                        )

        # after computing the intersecting voxels, prune them and add the certificates to a map
        bit_string_idx = 0
        while bit_string_idx < len(intersecting_areas):
            bit_string = intersecting_areas[bit_string_idx]
            # increase idx for the next iteration
            bit_string_idx += 1

            skip = False
            # iterate over all prefixes of that bitstring from largest/shortest to smallest/longest
            for i in range(1, len(bit_string)):
                # check if any of its prefixes (larger areas) is also part of intersecting_areas
                if bit_string[:i] in intersecting_areas:
                    # if it is, ignore this one as the certificate will be included in the larger/shorter
                    # prefix
                    skip = True
                    break

            if skip:
                # ignore by skipping over this index
                continue

            # check if area can be merged with neighbor
            bit_string_neighbor = (
                bit_string[:-1] + ("0" if bit_string[-1:] == "1" else "1")
            )
            if bit_string_neighbor in intersecting_areas:
                # yes it can. ignore current bit_string by skipping (continue)
                # but remove neighbor to prevent duplicate area
                intersecting_areas.remove(bit_string_neighbor)
                # and add parent at the end of the list to make sure duplicate test is performed with parent again
                intersecting_areas.append(bit_string[:-1])

                continue

            # from this point on bit_string is sucessfully taken

            if plot:
                x, y = ZOrderBitString.from_bit_string(
                    bit_string=bit_string
                ).to_shapely_area().exterior.xy

                plt.plot(x, y, color="g", linewidth=5)

            if bit_string in bit_string_map:
                bit_string_map[bit_string].certificate_hashes.add(
                    geo_cert.hash()
                )
            else:
                bit_string_map[bit_string] = BitStringRow(
                    bit_string=bit_string,
                    certificate_hashes=set([geo_cert.hash()])
                )

            # iterate over all prefixes of that bitstring and add them to bit_string_map
            for i in range(1, len(bit_string)):
                bit_string_prefix = bit_string[:i]
                if not (bit_string_prefix in bit_string_map):
                    # add an empty entry
                    bit_string_map[bit_string_prefix] = BitStringRow(
                        bit_string=bit_string_prefix,
                        certificate_hashes=set()
                    )

        if plot:
            plt.show()
            exit()

        # if row_i > 1:
        #     assert "1" in bit_string_map
        #     break
        # print(len(intersecting_areas))
        # exit()

    del df
    row = None
    df = pd.DataFrame()
    gc.collect()
    # sort by length so that we can start computing the hashes from the bottom of the tree
    bit_strings = sorted(bit_string_map.keys(), key=lambda x: (-len(x), x))
    for bit_string in tqdm(
        bit_strings,
        total=len(bit_strings),
        desc="compute hashes"
    ):
        row = bit_string_map[bit_string]

        bit_string_left_child = bit_string + "0"
        bit_string_right_child = bit_string + "1"

        row.left_child_hash = (
            None if len(bit_string) == 66 else
            (
                bit_string_map[bit_string_left_child].hash if bit_string_left_child in bit_string_map
                else DEFAULT_HASH
            )
        )

        row.right_child_hash = (
            None if len(bit_string) == 66 else
            (
                bit_string_map[bit_string_right_child].hash if bit_string_right_child in bit_string_map
                else DEFAULT_HASH
            )
        )

        # compute this node's hash
        if len(bit_string) == 66:
            # for leaves the hash is H(0 || H(C_0) || H(C_1) | ...)
            row.hash = hashlib.sha256(
                b"\x00" +
                row.get_certificate_hashes()
            ).digest()
        else:
            # for intermediate nodes H(1 || h_1 || h_2) if there are no certificate hashes
            # and H(1 || h_1 || h_2 || h_3) otherwise
            if len(row.certificate_hashes) > 0:
                row.hash = hashlib.sha256(
                    b"\x01" +
                    row.left_child_hash +
                    row.right_child_hash +
                    hashlib.sha256(row.get_certificate_hashes()).digest()
                ).digest()
            else:
                row.hash = hashlib.sha256(
                    b"\x01" +
                    row.left_child_hash +
                    row.right_child_hash
                ).digest()

        # flip last bit
        bit_string_neighbor = bit_string[:-1] + \
            ("0" if bit_string[-1:] == "1" else "1")

        if bit_string_neighbor in bit_string_map:
            # neighbor exists, set it's neighbor hash
            # when iterating over the neighbor, this row's neighbor hash will be set
            bit_string_map[bit_string_neighbor].neighbor_hash = row.hash
        else:
            # neighbor does not exist, own neighbor hash is set to the empty one
            row.neighbor_hash = DEFAULT_HASH

    f_geo_nodes = open(os.path.join(output_path, "geo-nodes.csv"), "w")
    f_geo_nodes.write(GEO_NODE_CSV_HEADER)

    f_left_child = open(os.path.join(output_path, "left-child.csv"), "w")
    f_left_child.write(LEFT_CHILD_CSV_HEADER)

    f_right_child = open(os.path.join(output_path, "right-child.csv"), "w")
    f_right_child.write(RIGHT_CHILD_CSV_HEADER)

    f_includes = open(os.path.join(output_path, "includes.csv"), "w")
    f_includes.write(INCLUDES_CSV_HEADER)

    # everything will be in one table, sort in lexicographical order
    # sort in z-order for insertion
    bit_strings.sort(key=lambda x: int(x, 2))

    for bit_string in tqdm(
        bit_strings,
        total=len(bit_strings),
        desc="write area"
    ):
        row = bit_string_map[bit_string]

        # string has to be of form POLYGON((lon lat, lon lat, ...))
        polygon = voxel_bounds_to_2d_wkt_polygon(
            ZOrderBitString.from_bit_string(bit_string).to_voxel_bounds()
        )

        neighbor_hash = None if row.neighbor_hash is None else f"{row.neighbor_hash.hex()}"

        bit_string_left_child = bit_string + "0"
        left_child_hash = None if row.left_child_hash is None else f"{row.left_child_hash.hex()}"

        if bit_string_left_child in bit_string_map:
            f_left_child.write(
                f"{bit_string},{bit_string_left_child}\n"
            )

        bit_string_right_child = bit_string + "1"
        right_child_hash = None if row.right_child_hash is None else f"{row.right_child_hash.hex()}"

        if bit_string_right_child in bit_string_map:
            f_right_child.write(
                f"{bit_string},{bit_string_right_child}\n"
            )

        f_geo_nodes.write(
            f"{bit_string},{neighbor_hash},{left_child_hash},{right_child_hash}\n"
        )

        certificate_hashes = [
            h.hex()
            for h in row.certificate_hashes
        ]

        for certificate_hash in certificate_hashes:
            f_includes.write(f"{bit_string},{certificate_hash}\n")

    f_geo_nodes.close()
    f_left_child.close()
    f_right_child.close()
    f_includes.close()

    f_certificates = open(os.path.join(output_path, "certificates.csv"), "w")
    f_certificates.write(CERTIFICATE_CSV_HEADER)

    for geo_certificate in tqdm(
        geo_certificates,
        total=len(geo_certificates),
        desc="write cert"
    ):

        certificate_hash = f"{geo_certificate.hash().hex()}"
        certificate = f"{geo_certificate.to_cert().hex()}"

        f_certificates.write(f"{certificate_hash}\n")

    f_certificates.close()


if __name__ == '__main__':
    main()
