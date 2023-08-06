from typing import Dict, Any, Union, Optional, List, Tuple
import click
import pandas as pd
import numpy as np
from shapely import Point, Polygon
from getpass import getpass
from tqdm import tqdm
from multiprocessing import Process, Event, Value
import time
import itertools
import sys
import os

import psycopg2

sys.path.insert(1, os.path.join(sys.path[0], '../../../../performance'))  # noqa - prevent auto formatting
from sampling import load_bit_string_sampling_map, sample_df


class ProcessArgs:
    def __init__(
            self,
            query_set: list[str],
            db_host: str,
            db_port: int,
            db_name: str,
            db_user: str,
            db_pass: str,
            query_radius: int,
            batch_size: int,
            excluding_bit_string_computation: bool,
            start_event: Event,
            ready_event: Event,
            stop_event: Event
    ) -> None:
        self.query_set = query_set
        self.db_host = db_host
        self.db_port = db_port
        self.db_name = db_name
        self.db_user = db_user
        self.db_pass = db_pass
        self.query_radius = query_radius
        self.batch_size = batch_size
        self.excluding_bit_string_computation = excluding_bit_string_computation
        self.start_event = start_event
        self.ready_event = ready_event
        self.stop_event = stop_event


def bitstrings_to_query(bit_strings: Tuple[str]):
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
    query_set = args.query_set

    if args.excluding_bit_string_computation:
        query_set = [
            bitstrings_to_query(q)
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
                bitstrings_to_query(q, args.query_radius)
                for q in queries
            ]

        cursor.execute(
            ";".join(
                [

                    (
                        f"SELECT bit_string_51, bit_string_15, certificate_hashes, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "
                        f"FROM nodes "
                        f"WHERE bit_string_51 IN (''," +
                        ','.join(
                            set(
                                itertools.chain.from_iterable(
                                    point_queries
                                    for point_queries, _, _ in bit_strings
                                )
                            )
                        ) + ") AND "
                        f"altitude_min <= 32767 AND "
                        f"altitude_max >= -1 UNION " +
                        "UNION".join(
                            [
                                "(SELECT bit_string_51, bit_string_15, certificate_hashes, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "
                                "FROM nodes "
                                f"WHERE "
                                f"bit_string_51_int >= {imin} AND "
                                f"bit_string_51_int <= {imax} AND "
                                f"altitude_min <= 32767 AND "
                                f"altitude_max >= -1"
                                f")"
                                for _, imin, imax in bit_strings
                            ]
                        )
                    )
                    for bit_strings in queries
                ]
            )
        )

        # simulate fetching all results
        res = cursor.fetchall()
        result_count_tmp = len(res)

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
    default=11  # ceil(10m * 1.005)
)
@click.option('--excluding-bit-string-computation', 'excluding_bit_string_computation', flag_value=True, default=False)
@click.option(
    '--batch-size',
    '-b',
    'batch_size',
    type=int,
    default=80
)
def main(
    sampling_map_path: str,
    db_host: str,
    db_port: int,
    db_name: str,
    db_user: str,
    db_pass: Optional[str],
    num_threads: int,
    time_s: int,
    query_radius: int,
    excluding_bit_string_computation: bool,
    batch_size: bool
):
    sampling_map = load_bit_string_sampling_map(sampling_map_path)

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

    queries_per_thread = len(sampling_map) // num_threads
    query_set = sampling_map["bit_strings"].values

    processes: List[Process] = [
        Process(
            target=run_queries,
            args=(
                ProcessArgs(
                    query_set=query_set[
                        i * queries_per_thread:
                        (i + 1) * queries_per_thread
                    ],
                    db_host=db_host,
                    db_port=db_port,
                    db_name=db_name,
                    db_user=db_user,
                    db_pass=db_pass,
                    query_radius=query_radius,
                    batch_size=batch_size,
                    excluding_bit_string_computation=excluding_bit_string_computation,
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
