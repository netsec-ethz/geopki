from typing import Dict, Any, Union, Optional, List, Tuple
import click
import pandas as pd
import numpy as np
from shapely import Point, Polygon
from getpass import getpass
from multiprocessing import Process, Event, Value
import time
import sys
import os

import psycopg2

sys.path.insert(1, os.path.join(sys.path[0], '../../../..'))  # noqa - prevent auto formatting
from coordinatez import GeodeticCoordinate, polygons_to_2d_bit_strings, sphere_to_polygon


def sample_point_in_polygon(polygon: Polygon) -> tuple[float, float]:
    "https://www.matecdev.com/posts/random-points-in-polygon.html"
    minX, minY, maxX, maxY = polygon.bounds

    while True:
        # rejection sampling
        sample = Point(np.random.uniform(minX, maxX),
                       np.random.uniform(minY, maxY))

        if polygon.contains(sample):
            return sample.x, sample.y


def generate_queries(query_count: int, df_website_density: pd.DataFrame) -> list[tuple[float, float, int]]:
    sample = df_website_density.sample(
        n=query_count,
        weights='weight',
        random_state=1,
        replace=True
    )

    # for each sample, sample a point within the polygon
    sample['sample_point'] = sample['polygon'].apply(sample_point_in_polygon)

    return [
        (longitude, latitude, 22767)
        for (longitude, latitude) in sample['sample_point'].values
    ]


class ProcessArgs:
    def __init__(
            self,
            df_website_density: pd.DataFrame,
            db_host: str,
            db_port: int,
            db_name: str,
            db_user: str,
            db_pass: str,
            query_radius: int,
            batch_size: int,
            count_only: bool,
            excluding_bit_string_computation: bool,
            query_set_size: int,
            start_event: Event,
            ready_event: Event,
            stop_event: Event
    ) -> None:
        self.df_website_density = df_website_density
        self.db_host = db_host
        self.db_port = db_port
        self.db_name = db_name
        self.db_user = db_user
        self.db_pass = db_pass
        self.query_radius = query_radius
        self.batch_size = batch_size
        self.count_only = count_only
        self.excluding_bit_string_computation = excluding_bit_string_computation
        self.query_set_size = query_set_size
        self.start_event = start_event
        self.ready_event = ready_event
        self.stop_event = stop_event


def query_to_bitstring_integers(query: Tuple[float, float, int], query_radius: float):
    longitude, latitude, altitude = query

    bit_strings = polygons_to_2d_bit_strings(
        polygons=[sphere_to_polygon(
            center=GeodeticCoordinate(
                longitude=longitude,
                latitude=latitude,
                altitude=altitude
            ),
            radius_m=query_radius
        )],
        f_grow=1,
        f_min=0
    )

    return [
        (
            # compute all prefixes of bit_string that are not obtained by removing a trailing zero
            [
                f"b'{b[:i]}'"
                for i in range(1, bl)
            ],
            int(bit_string.ljust(51, '0'), 2),
            int(bit_string.ljust(51, '1'), 2)
        )
        for bit_string in bit_strings
        # define local variable, requires python >= 3.8 (https://stackoverflow.com/a/55881984)
        if (b := bit_string.rstrip("0")) and (bl := len(b))
    ]


def run_queries(args: ProcessArgs, executed_queries_value: Value, result_count_value: Value):
    result_count = 0
    executed_queries = 0
    query_idx = 0

    conn = psycopg2.connect(
        host=args.db_host,
        port=args.db_port,
        database=args.db_name,
        user=args.db_user,
        password=args.db_pass
    )

    cursor = conn.cursor()

    # pre-generate a query set
    query_set = generate_queries(
        args.query_set_size,
        args.df_website_density
    )

    if args.excluding_bit_string_computation:
        query_set = [
            query_to_bitstring_integers(q, args.query_radius)
            for q in query_set
        ]

    # signal we're ready
    args.ready_event.set()

    # wait for start signal
    args.start_event.wait()

    # work until stop is signalled
    while True:
        query_idx = (query_idx + args.batch_size) % len(query_set)
        queries = query_set[
            query_idx:(query_idx + args.batch_size)
        ]

        # execute query / queries

        if not args.excluding_bit_string_computation:
            queries = [
                query_to_bitstring_integers(q, args.query_radius)
                for q in queries
            ]

        cursor.execute(
            ";".join(
                [

                    (
                        ("SELECT COUNT(*) FROM (" if args.count_only else "") +
                        "UNION".join(
                            [
                                f"(SELECT bit_string_51, bit_string_15, certificate_hashes, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "
                                f"FROM nodes "
                                f"WHERE bit_string_51 IN (" +
                                ','.join(point_queries) + ") AND "
                                f"altitude_min <= {query_altitude + args.query_radius} AND "
                                f"altitude_max >= {query_altitude - args.query_radius}"
                                "UNION ALL "
                                "SELECT bit_string_51, bit_string_15, certificate_hashes, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "
                                "FROM nodes "
                                f"WHERE "
                                f"bit_string_51_int >= {imin} AND "
                                f"bit_string_51_int <= {imax} AND "
                                # fix altitude for now
                                f"altitude_min <= {query_altitude + args.query_radius} AND "
                                f"altitude_max >= {query_altitude - args.query_radius}"
                                f")"
                                for point_queries, imin, imax in bit_strings
                                if (query_altitude := 22767)
                            ])
                        + (") as sq" if args.count_only else "")
                    )
                    for bit_strings in queries
                ]
            )
        )

        # simulate fetching all results
        res = cursor.fetchall()
        result_count_tmp = res[0][0] if args.count_only else len(res)
        res = None

        # check whether we need to stop
        if args.stop_event.is_set():
            break

        # increase result and query counters
        result_count += result_count_tmp
        executed_queries += args.batch_size

    cursor.close()
    conn.close()

    executed_queries_value.value = executed_queries
    result_count_value.value = result_count


@click.command()
@click.option(
    '--website-density',
    '-w',
    'website_density_path',
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
@click.option(
    '--threads',
    '-m',
    'num_threads',
    type=int,
    default=2
)
@click.option(
    '--time',
    '-t',
    'time_s',
    type=int,
    default=10
)
@click.option(
    '--query-radius',
    '-r',
    'query_radius',
    type=int,
    default=10
)
@click.option(
    '--qps-set-size',
    '-q',
    'qps_set_size',
    type=int,
    default=1000
)
@click.option('--excluding-bit-string-computation', 'excluding_bit_string_computation', flag_value=True, default=False)
@click.option('--count-only', 'count_only', flag_value=True, default=False)
@click.option(
    '--batch-size',
    '-b',
    'batch_size',
    type=int,
    default=80
)
def main(
    website_density_path: str,
    db_host: str,
    db_port: int,
    db_name: str,
    db_user: str,
    db_pass: Optional[str],
    num_threads: int,
    time_s: int,
    query_radius: int,
    qps_set_size: int,
    excluding_bit_string_computation: bool,
    count_only: bool,
    batch_size: bool
):
    if not website_density_path.endswith(".parquet"):
        raise Exception(f"website density path does to end in '.parquet'")

    df_website_density = pd.read_parquet(website_density_path)

    df_website_density['polygon'] = df_website_density['polygon'].apply(
        lambda points:
        Polygon([
            (p[1], p[0])
            for p in points
        ])
    )

    # probability 0 if osm_website_element_count == 0
    df_website_density['weight'] = df_website_density['osm_website_element_count']

    # very small probability if osm_website_element_count == 0
    # df['weights'] = df['osm_website_element_count'] + 1

    if db_pass is None:
        db_pass = getpass(
            f"Password for {db_user}:{db_name}@{db_host}:{db_port}: "
        )

    start_event = Event()
    stop_event = Event()

    ready_events = [
        Event()
        for _ in range(num_threads)
    ]

    executed_queries_values = [
        Value("i", 0, lock=False)
        for _ in range(num_threads)
    ]
    result_count_values = [
        Value("i", 0, lock=False)
        for _ in range(num_threads)
    ]

    processes: List[Process] = [
        Process(
            target=run_queries,
            args=(
                ProcessArgs(
                    df_website_density=df_website_density,
                    db_host=db_host,
                    db_port=db_port,
                    db_name=db_name,
                    db_user=db_user,
                    db_pass=db_pass,
                    query_radius=query_radius,
                    batch_size=batch_size,
                    count_only=count_only,
                    excluding_bit_string_computation=excluding_bit_string_computation,
                    query_set_size=int(qps_set_size * time_s / num_threads),
                    start_event=start_event,
                    ready_event=ready_event,
                    stop_event=stop_event,
                ),
                executed_queries_values,
                result_count_value
            )
        )
        for i, ready_event, executed_queries_values, result_count_value
        in zip(
            range(num_threads),
            ready_events,
            executed_queries_values,
            result_count_values
        )
    ]

    # initiate all threads
    for process in processes:
        process.start()

    # wait for all process to be ready
    for ready_event in ready_events:
        ready_event.wait()

    # give start signal
    start_event.set()
    # wait for the given time
    time.sleep(time_s)
    # then signal termination to the threads
    stop_event.set()

    # wait for all of them to terminate
    for process in processes:
        process.join()

    executed_queries = 0
    result_count = 0

    for xq, rc in zip(executed_queries_values, result_count_values):
        # read out result
        executed_queries += xq.value
        result_count += rc.value

    print(f"{num_threads},{time_s},{query_radius},{executed_queries},{result_count}")


if __name__ == '__main__':
    main()
