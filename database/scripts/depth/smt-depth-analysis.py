from typing import Optional, Tuple
import click
from getpass import getpass
from tqdm import tqdm
import itertools
import sys
import os

import psycopg2

sys.path.insert(1, os.path.join(sys.path[0], '../../../performance'))  # noqa - prevent auto formatting
sys.path.insert(1, os.path.join(sys.path[0], '../../..'))  # noqa - prevent auto formatting
from coordinatez import DiscretizedVoxel, GeodeticCoordinate
from sampling import load_sampling_map, sample_df


def query_to_bitstring(query: Tuple[float, float]):
    longitude, latitude = query

    bit_strings = [
        DiscretizedVoxel.from_coordinate(
            GeodeticCoordinate(
                longitude=longitude,
                latitude=latitude,
                altitude=0
            )
        ).to_bit_string_tuple()[0]
    ]

    assert len(bit_strings) == 1
    assert len(bit_strings[0]) == 51

    bit_string = bit_strings[0]
    b = bit_string.rstrip("0")
    bl = len(b)

    return (
        # compute all prefixes of bit_string that are not obtained by removing a trailing zero
        [
            f"'{b[:i]}'"
            for i in range(1, bl)
        ],
        int(bit_string, 2)
    )


@click.command()
@click.argument('output_path', type=click.Path(exists=False))
@click.option(
    '--sampling-map',
    '-w',
    'sampling_map_path',
    type=str,
    required=True
)
@click.option(
    '--db-host',
    '-h',
    'db_host',
    type=str,
    required=True
)
@click.option(
    '--db-port',
    '-p',
    'db_port',
    type=int,
    required=True
)
@click.option(
    '--db-name',
    '-n',
    'db_name',
    type=str,
    required=True
)
@click.option(
    '--db-user',
    '-u',
    'db_user',
    type=str,
    required=True
)
@click.option(
    '--db-pass',
    '-u',
    'db_pass',
    type=str,
    required=False
)
def main(
    output_path: str,
    sampling_map_path: str,
    db_host: str,
    db_port: int,
    db_name: str,
    db_user: str,
    db_pass: Optional[str],
):
    sampling_map = load_sampling_map(sampling_map_path)

    if db_pass is None:
        db_pass = getpass(
            f"Password for {db_user}:{db_name}@{db_host}:{db_port}: "
        )

    conn = psycopg2.connect(
        host=db_host,
        port=db_port,
        database=db_name,
        user=db_user,
        password=db_pass
    )

    cursor = conn.cursor()

    if os.path.exists(output_path):
        raise Exception("output path already exists")
    else:
        f = open(output_path, "w")
        f.write(f"longitude,latitude,smt_depth_xy,smt_depth_z\n")
        f.flush()

    # work until stop is signalled
    for longitude, latitude in tqdm(
        sampling_map.values,
        total=len(sampling_map),
        desc="measure SMT depth"
    ):
        point_queries, bit_string_int = query_to_bitstring(
            (longitude, latitude)
        )
        # print(len(bit_strings))
        # print(bit_strings)
        # exit()

        # execute query
        cursor.execute(
            f"WITH sq AS ("
            f"SELECT LENGTH(bit_string_51) as depth_xy, MAX(LENGTH(bit_string_15)) as depth_z, LENGTH(bit_string_51) + MAX(LENGTH(bit_string_15)) as depth "
            f"FROM nodes "
            f"WHERE bit_string_51 IN (''," +
            ','.join(set(point_queries)) +
            ") "
            "GROUP BY bit_string_51"
            "), "
            "sq_max AS (SELECT MAX(sq.depth) as max_depth FROM sq) "
            "SELECT sq.depth_xy, sq.depth_z FROM sq, sq_max WHERE sq.depth=sq_max.max_depth"
        )

        # simulate fetching all results
        res = cursor.fetchall()
        smt_depth_xy = res[0][0]
        smt_depth_z = res[0][1]

        f.write(
            f"{longitude},{latitude},{smt_depth_xy},{smt_depth_z}\n"
        )

    cursor.close()
    conn.close()
    f.close()


if __name__ == '__main__':
    main()
