import click
import pandas as pd
import numpy as np
from tqdm import tqdm
from typing import Dict, List, Tuple, Optional, Set
from shapely import Polygon, MultiPolygon, box, to_geojson
from coordinates import ZOrderBitString, GeodeticCoordinate
from collections import deque
import matplotlib.pyplot as plt
import json
import os
import math
import gc
import hashlib

INITIAL_AREA_FRACTION = 1
MAX_FILE_SIZE = 300 * 1000 * 1000  # 300 MB

INSERT_INTO_NODES_STR = (
    f"INSERT INTO nodes (" +
    ",".join([
        "bit_string",
        "area",
        "neighbor_hash",
        "left_child_hash",
        "right_child_hash",
        "certificate_hashes"
    ]) +
    f") VALUES\n"
)
INSERT_INTO_NODES_BITSTRING_INT_STR = (
    f"INSERT INTO nodes (" +
    ",".join([
        "bit_string",
        "bit_string_51",
        "neighbor_hash",
        "left_child_hash",
        "right_child_hash",
        "certificate_hashes"
    ]) +
    f") VALUES\n"
)
INSERT_INTO_NODES_BITSTRING_INT_Z_SUBTREES_STR = (
    f"INSERT INTO nodes (" +
    ",".join([
        "bit_string_51",
        "bit_string_51_int",
        "bit_string_15",
        "neighbor_hash",
        "xy_left_child_hash",
        "xy_right_child_hash",
        "z_left_child_hash",
        "z_right_child_hash",
        "certificate_hashes"
    ]) +
    f") VALUES\n"
)
INSERT_INTO_CERTS_STR = (
    f"INSERT INTO certificates (" +
    ",".join([
        "certificate_hash",
        "certificate"
    ]) +
    f") VALUES\n"
)


class GeoCertificate:
    def __init__(
        self,
        domain: str,
        area: MultiPolygon,
        area_id: str,
        parents: np.ndarray,
        children: np.ndarray,
    ) -> None:

        self.domain = domain
        self.area = area
        self.area_id = area_id
        self.parents: List[str] = parents.tolist()
        self.children: List[str] = children.tolist()

    def to_cert(self) -> bytes:
        return json.dumps(
            {
                'domain': self.domain,
                'area': json.loads(to_geojson(self.area)),
                'area_id': self.area_id,
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

        self.xy_left_child_hash: Optional[bytes] = None
        self.xy_right_child_hash: Optional[bytes] = None
        self.z_left_child_hash: Optional[bytes] = None
        self.z_right_child_hash: Optional[bytes] = None
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
@click.argument('output_path_nodes', type=click.Path(exists=False))
@click.argument('output_path_certificates', type=click.Path(exists=False))
@click.option('--plot', 'plot', flag_value=True, default=False)
@click.option('--bitstring', 'mode', flag_value='bitstring-int', default='bitstring-int')
@click.option('--spatial', 'mode', flag_value='spatial')
def main(
    input_path: str,
    output_path_nodes: str,
    output_path_certificates: str,
    plot: bool,
    mode: str,
):

    if not mode in ["bitstring-int", "spatial"]:
        raise Exception(f"Unsupported mode '{mode}'")

    if input_path.endswith(".parquet"):
        df = pd.read_parquet(input_path)
    elif input_path.endswith(".csv"):
        df = pd.read_csv(input_path)
    else:
        raise Exception(f"Unkown input file type")

    if not os.path.isdir(output_path_nodes):
        raise Exception(f"Node output path has to be a directory")

    if not os.path.isdir(output_path_certificates):
        raise Exception(f"Certificate output path has to be a directory")

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
        # if row['area_id'] != "rel:270056":
        #     continue

        # ethz
        # if not "way:192151232" in row['area_id']:
        #     continue

        # 'area_id', 'polygons', 'parents', 'children', 'domain'
        valid_polygons = [
            polygon
            for polygon in row['polygons']
            # exclude not proper areas
            if polygon[0] == polygon[-1]
        ]

        if len(valid_polygons) <= 0:
            continue

        area_id = row['area_id']

        # China
        # if area_id == "rel:270056":
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
            area_id=area_id,
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

        row.xy_left_child_hash = (
            None if len(bit_string) == 66 else
            (
                bit_string_map[bit_string_left_child].hash if bit_string_left_child in bit_string_map
                else hashlib.sha256(b"\x00").digest()
            )
        )

        row.xy_right_child_hash = (
            None if len(bit_string) == 66 else
            (
                bit_string_map[bit_string_right_child].hash if bit_string_right_child in bit_string_map
                else hashlib.sha256(b"\x00").digest()
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
                    row.xy_left_child_hash +
                    row.xy_right_child_hash +
                    hashlib.sha256(row.get_certificate_hashes()).digest()
                ).digest()
            else:
                row.hash = hashlib.sha256(
                    b"\x01" +
                    row.xy_left_child_hash +
                    row.xy_right_child_hash
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
            row.neighbor_hash = hashlib.sha256(b"\x00").digest()

    f = open(os.path.join(output_path_nodes, "part-0.sql"), "w")
    size = 0

    is_first_line = True
    i = 0

    # sort in z-order for insertion to facilitate manual inspection of the output
    bit_strings.sort(key=lambda x: int(x, 2))

    for bit_string in tqdm(
        bit_strings,
        total=len(bit_strings),
        desc="write nodes"
    ):
        row = bit_string_map[bit_string]

        # string has to be of form POLYGON((lon lat, lon lat, ...))
        polygon = voxel_bounds_to_2d_wkt_polygon(
            ZOrderBitString.from_bit_string(bit_string).to_voxel_bounds()
        )

        bit_string_51_int = int(
            bit_string[:51].ljust(51, '0'),
            2
        )
        neighbor_hash = "NULL" if row.neighbor_hash is None else f"E'\\\\x{row.neighbor_hash.hex()}'"
        xy_left_child_hash = "NULL" if row.xy_left_child_hash is None else f"E'\\\\x{row.xy_left_child_hash.hex()}'"
        xy_right_child_hash = "NULL" if row.xy_right_child_hash is None else f"E'\\\\x{row.xy_right_child_hash.hex()}'"
        z_left_child_hash = "NULL" if row.z_left_child_hash is None else f"E'\\\\x{row.z_left_child_hash.hex()}'"
        z_right_child_hash = "NULL" if row.z_right_child_hash is None else f"E'\\\\x{row.z_right_child_hash.hex()}'"
        certificate_hashes = f",".join(
            [
                f"E'\\\\x{h.hex()}'::bytea"
                for h in row.certificate_hashes
            ]
        )
        certificate_hash_array = f"ARRAY[{certificate_hashes}]::bytea[]"

        # don't add comma on the first line
        if is_first_line:
            if mode == "bitstring-int":
                size += f.write(INSERT_INTO_NODES_BITSTRING_INT_STR)
            elif mode == "bitstring-int-z-subtrees":
                size += f.write(INSERT_INTO_NODES_BITSTRING_INT_Z_SUBTREES_STR)
            elif mode == "spatial":
                size += f.write(INSERT_INTO_NODES_STR)

            is_first_line = False
        else:
            size += f.write(",\n")

        if mode == "bitstring-int":
            size += f.write(
                f"('{bit_string}', {bit_string_51_int}, {neighbor_hash}, {xy_left_child_hash}, {xy_right_child_hash}, {certificate_hash_array})"
            )
        elif mode == "bitstring-int-z-subtrees":
            size += f.write(
                f"(b'{bit_string[:51]}', {bit_string_51_int}, b'{bit_string[51:]}', {neighbor_hash}, {xy_left_child_hash}, {xy_right_child_hash}, {z_left_child_hash}, {z_right_child_hash}, {certificate_hash_array})"
            )
        else:
            size += f.write(
                f"(b'{bit_string}', {polygon}, {neighbor_hash}, {xy_left_child_hash}, {xy_right_child_hash}, {certificate_hash_array})"
            )

        if size >= MAX_FILE_SIZE:
            size += f.write("\nON CONFLICT (bit_string) DO NOTHING")
            f.close()
            i += 1

            f = open(os.path.join(output_path_nodes, f"part-{i}.sql"), "w")

            # reset size
            size = 0
            is_first_line = True

    size += f.write("\nON CONFLICT (bit_string) DO NOTHING")
    f.close()

    f = open(os.path.join(output_path_certificates, "part-0.sql"), "w")
    size = 0

    is_first_line = True
    i = 0

    for geo_certificate in tqdm(
        geo_certificates,
        total=len(geo_certificates),
        desc="write cert"
    ):
        # don't add comma on the first line
        if is_first_line:
            size += f.write(INSERT_INTO_CERTS_STR)
            is_first_line = False
        else:
            size += f.write(",\n")

        certificate_hash = f"E'\\\\x{geo_certificate.hash().hex()}'"
        certificate = f"E'\\\\x{geo_certificate.to_cert().hex()}'"

        size += f.write(
            f"({certificate_hash}, {certificate})"
        )

        if size >= MAX_FILE_SIZE:
            size += f.write("\nON CONFLICT (certificate_hash) DO NOTHING")
            f.close()
            i += 1

            f = open(
                os.path.join(output_path_certificates, f"part-{i}.sql"),
                "w"
            )

            # reset size
            size = 0
            is_first_line = True

    size += f.write("\nON CONFLICT (certificate_hash) DO NOTHING")
    f.close()


if __name__ == '__main__':
    main()
