import click
import pandas as pd
import numpy as np
from tqdm import tqdm
from typing import Dict, List, Tuple, Optional, Set
from shapely import Polygon, MultiPolygon, box, to_geojson
import json
import os
import sys
import gc
import hashlib

sys.path.insert(1, os.path.join(sys.path[0], '../../..'))  # noqa - prevent auto formatting
# from coordinates import ZOrderBitString, GeodeticCoordinate, extruded_polygons_to_bit_strings, polygons_to_2d_bit_strings
from coordinatez import DiscretizedVoxel, GeodeticCoordinate, extruded_polygons_to_bit_string_tuples

INITIAL_AREA_FRACTION = 0.1
MAX_FILE_SIZE = 300 * 1000 * 1000  # 300 MB
MAX_CERTIFICATE_SIZE = 3328        # 3.25KiB / 99% is below this

DEFAULT_HASH = hashlib.sha256(b"\x00").digest()

INSERT_INTO_NODES_STR = (
    f"INSERT INTO nodes (" +
    ",".join([
        "bit_string_51",
        "bit_string_15",
        "area",
        "xy_left_child_hash",
        "xy_right_child_hash",
        "z_left_child_hash",
        "z_right_child_hash",
        "certificate_hashes"
    ]) +
    f") VALUES\n"
)
INSERT_INTO_NODES_BITSTRING_INT_Z_SUBTREES_STR = (
    f"INSERT INTO nodes (" +
    ",".join([
        "bit_string_51",
        "bit_string_15",
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
        "certificate",
        "not_valid_after"
    ]) +
    f") VALUES\n"
)


class GeoCertificate:
    def __init__(
        self,
        domain: str,
        certificate_id: str,
        not_valid_after: str,
        list_of_multipolygons: List[List[List[Dict[str, float]]]],
        list_of_altitudes: List[Tuple[float, float]],
        parents: np.ndarray,
        children: np.ndarray,
    ) -> None:

        self.domain = domain
        self.certificate_id = certificate_id
        self.not_valid_after = not_valid_after
        self.list_of_multipolygons = list_of_multipolygons
        self.list_of_altitudes = list_of_altitudes
        self.parents: List[str] = parents.tolist()
        self.children: List[str] = children.tolist()

    def to_cert(self) -> bytes:
        return json.dumps(
            {
                'domain': self.domain,
                'areas': [
                    json.loads(
                        to_geojson(
                            MultiPolygon(
                                [
                                    Polygon(
                                        [(x['lon'], x['lat']) for x in polygon]
                                    )
                                    for polygon in multipolygon
                                ]
                            )
                        )
                    )
                    for multipolygon in self.list_of_multipolygons
                ],
                'areas_altitude': list(self.list_of_altitudes),
                'certificate_id': self.certificate_id,
                'not_valid_after': self.not_valid_after,
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
            certificate_hashes: Set[bytes],
    ) -> None:

        self.certificate_hashes = certificate_hashes

        self.xy_left_child_hash: Optional[bytes] = None
        self.xy_right_child_hash: Optional[bytes] = None
        self.z_left_child_hash: Optional[bytes] = None
        self.z_right_child_hash: Optional[bytes] = None
        self.hash = None

    def get_concatenated_certificate_hashes(self) -> bytes:
        return b"".join(sorted(self.certificate_hashes))

    def get_certificate_hashes(self) -> list[bytes]:
        return sorted(self.certificate_hashes)


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


def level_to_altitude(
    min_level: Optional[str],
    max_level: Optional[str],
    level: str,
    surface_altitude: Optional[float]
) -> Tuple[float, float]:

    # if surface_geodetic_altitude is None -> 0
    surface_altitude = 0 if surface_altitude is None else surface_altitude

    if level == "@":
        # is a node
        # no floors in this building, just use the full height
        assert min_level == '@'
        assert max_level == '@'

        return DiscretizedVoxel.D, DiscretizedVoxel.H
    elif level == '':
        # is a building / area
        # if there are nodes within the building, min_level and max_level
        # might be set

        if min_level == '@' and max_level == '@':
            # all floors span the full building height
            # --> don't have any reference points
            return DiscretizedVoxel.D, DiscretizedVoxel.H
        elif min_level == '' and max_level == '':
            # some building / area without any areas / nodes within in
            return DiscretizedVoxel.D, DiscretizedVoxel.H
        else:
            # span the full height given by min_level and max_level
            min_level = float(min_level)
            max_level = float(max_level)

            # for now just assume one level is three meters
            # bound check in case of weirdly formatted data
            return min(
                max(
                    surface_altitude + min_level * 3,
                    DiscretizedVoxel.D
                ),
                DiscretizedVoxel.H
            ), min(
                max(
                    surface_altitude + (max_level + 1) * 3,
                    DiscretizedVoxel.D
                ),
                DiscretizedVoxel.H
            )
    else:
        # use float(), apparently there is floor -0.5 in the dataset
        try:
            level = float(level)
            min_level = float(min_level)
            max_level = float(max_level)

            assert min_level <= level and level <= max_level

            # for now just assume one level is three meters
            # bound check in case of weirdly formatted data
            return (
                min(
                    max(
                        surface_altitude + level * 3,
                        DiscretizedVoxel.D
                    ),
                    DiscretizedVoxel.H
                ),
                min(
                    max(
                        surface_altitude + (level + 1) * 3,
                        DiscretizedVoxel.D
                    ),
                    DiscretizedVoxel.H
                )
            )

        except ValueError:
            print(level, min_level, max_level)
            raise


@click.command()
# the domain-location file
@click.argument('input_path', type=click.Path(exists=True))
# The place to store the output
@click.argument('output_path_nodes', type=click.Path(exists=False))
@click.argument('output_path_certificates', type=click.Path(exists=False))
@click.option('--bitstring-zsub', 'mode', flag_value='bitstring-int-z-subtrees', default='bitstring-int-z-subtrees')
@click.option('--spatial', 'mode', flag_value='spatial')
def main(
    input_path: str,
    output_path_nodes: str,
    output_path_certificates: str,
    mode: str,
):

    if not mode in ["bitstring-int-z-subtrees", "spatial"]:
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

    # in case individual bit strings and not bit string tuples are used,
    # the second string in the tuple must be set to ''
    bit_string_map: Dict[Tuple[str, str], BitStringRow] = {}

    for row_i, (idx, row) in tqdm(
        enumerate(df.iterrows()),
        total=len(df),
        desc="locate buildings"
    ):

        domain = row['domain']

        if domain is None or not isinstance(domain, str) or domain.strip() == '':
            continue

        # china
        # if row['certificate_id'] != "rel:270056":
        #     continue

        # ethz
        # if not "way:192151232" in row['certificate_id']:
        #     continue

        # 'certificate_id', 'list_of_multipolygons', 'list_of_levels', 'domain', 'min_building_level', 'max_building_level', 'parents', 'children'

        # China
        # if certificate_id == "rel:270056":
        #     plot = True
        certificate_id = row['certificate_id']
        list_of_multipolygons = row['list_of_multipolygons']
        list_of_levels = row['list_of_levels']
        parents = row['parents']
        children = row['children']
        min_level = row['min_building_level']
        max_level = row['max_building_level']
        surface_altitude = row['surface_altitude_aster_30'] if 'surface_altitude_aster_30' in df.columns else None
        # fixed expiration date
        not_valid_after = "2030-01-01 00:00:00+00"

        assert len(list_of_multipolygons) == len(list_of_levels)

        list_of_altitudes: list[tuple[float, float]] = []

        for level in list_of_levels:
            try:
                altitude_min, altitude_max = level_to_altitude(
                    min_level,
                    max_level,
                    level,
                    surface_altitude=surface_altitude
                )
            except (ValueError, AssertionError):
                print(row)
                raise

            list_of_altitudes.append((altitude_min, altitude_max))

        geo_cert = GeoCertificate(
            domain=domain,
            certificate_id=certificate_id,
            not_valid_after=not_valid_after,
            list_of_multipolygons=list_of_multipolygons,
            list_of_altitudes=list_of_altitudes,
            parents=parents,
            children=children,
        )

        if len(geo_cert.to_cert()) > MAX_CERTIFICATE_SIZE:
            continue

        geo_certificates.append(
            geo_cert
        )

        for multipolygon, (altitude_min, altitude_max) in zip(list_of_multipolygons, list_of_altitudes):

            shapely_polygons = [
                Polygon(
                    [(x['lon'], x['lat']) for x in polygon]
                )
                for polygon in multipolygon
            ]

            for xy_bit_string, z_bit_string in extruded_polygons_to_bit_string_tuples(
                polygons=shapely_polygons,
                altitude_min=altitude_min,
                altitude_max=altitude_max,
                f_grow=INITIAL_AREA_FRACTION
            ):
                if (xy_bit_string, z_bit_string) in bit_string_map:
                    bit_string_map[(xy_bit_string, z_bit_string)].certificate_hashes.add(
                        geo_cert.hash()
                    )
                else:
                    bit_string_map[(xy_bit_string, z_bit_string)] = BitStringRow(
                        certificate_hashes=set([geo_cert.hash()])
                    )

                # iterate over all prefixes of that bit string and add them to bit_string_map
                # first iterate over prefixes of z_bit_string, including the empty string ''
                for i in range(0, len(z_bit_string)):
                    z_bit_string_prefix = z_bit_string[:i]
                    if not ((xy_bit_string, z_bit_string_prefix) in bit_string_map):
                        # add an empty entry
                        bit_string_map[(xy_bit_string, z_bit_string_prefix)] = BitStringRow(
                            certificate_hashes=set()
                        )

                # next iterate over prefixes of xy_bit_string
                for i in range(0, len(xy_bit_string)):
                    xy_bit_string_prefix = xy_bit_string[:i]
                    if not ((xy_bit_string_prefix, '') in bit_string_map):
                        # add an empty entry
                        bit_string_map[(xy_bit_string_prefix, '')] = BitStringRow(
                            certificate_hashes=set()
                        )

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
    bit_strings = sorted(
        bit_string_map.keys(),
        key=lambda x: (-len(x[0]), -len(x[1]), x[0], x[1])
    )
    for xy_bit_string, z_bit_string in tqdm(
        bit_strings,
        total=len(bit_strings),
        desc="compute hashes"
    ):

        row = bit_string_map[(xy_bit_string, z_bit_string)]

        xy_left_child = (
            xy_bit_string + "0", ''
        ) if len(z_bit_string) == 0 else (
            # some inexistent node
            "a", ""
        )

        xy_right_child = (
            xy_bit_string + "1", ''
        ) if len(z_bit_string) == 0 else (
            # some inexistent node
            "a", ""
        )

        z_left_child = xy_bit_string, z_bit_string + "0"
        z_right_child = xy_bit_string, z_bit_string + "1"

        row.xy_left_child_hash = (
            bit_string_map[xy_left_child].hash if xy_left_child in bit_string_map
            else DEFAULT_HASH
        )

        row.xy_right_child_hash = (
            bit_string_map[xy_right_child].hash if xy_right_child in bit_string_map
            else DEFAULT_HASH
        )

        row.z_left_child_hash = (
            bit_string_map[z_left_child].hash if z_left_child in bit_string_map
            else DEFAULT_HASH
        )

        row.z_right_child_hash = (
            bit_string_map[z_right_child].hash if z_right_child in bit_string_map
            else DEFAULT_HASH
        )

        # compute this node's hash
        if len(xy_bit_string) == 51 and len(z_bit_string) == 15:
            # for leaves the hash is H(0 || H(C_0) || H(C_1) | ...)
            row.hash = hashlib.sha256(
                b"\x00" +
                row.get_concatenated_certificate_hashes()
            ).digest()
        else:
            # for intermediate nodes H(1 || h_1 || h_2) if there are no certificate hashes
            # and H(1 || h_1 || h_2 || h_3) otherwise

            if len(row.certificate_hashes) > 0:
                row.hash = hashlib.sha256(
                    b"\x01" +
                    row.xy_left_child_hash +
                    row.xy_right_child_hash +
                    row.z_left_child_hash +
                    row.z_right_child_hash +
                    hashlib.sha256(
                        row.get_concatenated_certificate_hashes()).digest()
                ).digest()
            else:
                row.hash = hashlib.sha256(
                    b"\x01" +
                    row.xy_left_child_hash +
                    row.xy_right_child_hash +
                    row.z_left_child_hash +
                    row.z_right_child_hash
                ).digest()

    f = open(os.path.join(output_path_nodes, "part-0.sql"), "w")
    size = 0

    is_first_line = True
    i = 0

    for bit_string_tuple in tqdm(
        bit_strings,
        total=len(bit_strings),
        desc="write nodes"
    ):
        row = bit_string_map[bit_string_tuple]

        # string has to be of form POLYGON((lon lat, lon lat, ...))
        polygon = voxel_bounds_to_2d_wkt_polygon(
            DiscretizedVoxel.from_bit_string_tuple(
                bit_string_tuple[0], bit_string_tuple[1]
            ).to_voxel_bounds()
        )

        xy_left_child_hash = "NULL" if (row.xy_left_child_hash is None or row.xy_left_child_hash ==
                                        DEFAULT_HASH) else f"E'\\\\x{row.xy_left_child_hash.hex()}'"
        xy_right_child_hash = "NULL" if (row.xy_right_child_hash is None or row.xy_right_child_hash ==
                                         DEFAULT_HASH) else f"E'\\\\x{row.xy_right_child_hash.hex()}'"
        z_left_child_hash = "NULL" if (row.z_left_child_hash is None or row.z_left_child_hash ==
                                       DEFAULT_HASH) else f"E'\\\\x{row.z_left_child_hash.hex()}'"
        z_right_child_hash = "NULL" if (row.z_right_child_hash is None or row.z_right_child_hash ==
                                        DEFAULT_HASH) else f"E'\\\\x{row.z_right_child_hash.hex()}'"
        certificate_hashes = f",".join(
            [
                f"E'\\\\x{h.hex()}'::bytea"
                for h in row.get_certificate_hashes()
            ]
        )
        certificate_hash_array = f"ARRAY[{certificate_hashes}]::bytea[]"

        # don't add comma on the first line
        if is_first_line:
            if mode == "bitstring-int-z-subtrees":
                size += f.write(INSERT_INTO_NODES_BITSTRING_INT_Z_SUBTREES_STR)
            elif mode == "spatial":
                size += f.write(INSERT_INTO_NODES_STR)

            is_first_line = False
        else:
            size += f.write(",\n")

        if mode == "bitstring-int-z-subtrees":
            altitude_min = int(bit_string_tuple[1].ljust(15, "0"), 2)
            altitude_max = int(bit_string_tuple[1].ljust(15, "1"), 2) + 1

            size += f.write(
                f"(b'{bit_string_tuple[0]}', b'{bit_string_tuple[1]}', {xy_left_child_hash}, {xy_right_child_hash}, {z_left_child_hash}, {z_right_child_hash}, {certificate_hash_array})"
            )
        else:
            size += f.write(
                f"(b'{bit_string_tuple[0]}', b'{bit_string_tuple[1]}', {polygon}, {xy_left_child_hash}, {xy_right_child_hash}, {z_left_child_hash}, {z_right_child_hash}, {certificate_hash_array})"
            )

        if size >= MAX_FILE_SIZE:
            # size += f.write("\nON CONFLICT (bit_string) DO NOTHING")
            f.close()
            i += 1

            f = open(os.path.join(output_path_nodes, f"part-{i}.sql"), "w")

            # reset size
            size = 0
            is_first_line = True

    # size += f.write("\nON CONFLICT (bit_string) DO NOTHING")
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
        expiration_date = f"'{geo_certificate.not_valid_after}'"

        size += f.write(
            f"({certificate_hash}, {certificate}, {expiration_date})"
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
