from typing import Dict, Any, Union, Optional, List, Tuple
import click
import pandas as pd
import numpy as np
from shapely import Point, Polygon
from getpass import getpass
from multiprocessing import Process, Event, Value
import time
import os
import sys
import psycopg2

sys.path.insert(1, os.path.join(sys.path[0], '../../../../performance'))  # noqa - prevent auto formatting
from sampling import load_sampling_map, sample_df


class ProcessArgs:
    def __init__(
            self,
            sampling_map: pd.DataFrame,
            db_host: str,
            db_port: int,
            db_name: str,
            db_user: str,
            db_pass: str,
            query_radius: int,
            batch_size: int,
            count_only: bool,
            query_set_size: int,
            start_event: Event,
            ready_event: Event,
            stop_event: Event
    ) -> None:
        self.sampling_map = sampling_map
        self.db_host = db_host
        self.db_port = db_port
        self.db_name = db_name
        self.db_user = db_user
        self.db_pass = db_pass
        self.query_radius = query_radius
        self.batch_size = batch_size
        self.count_only = count_only
        self.query_set_size = query_set_size
        self.start_event = start_event
        self.ready_event = ready_event
        self.stop_event = stop_event


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
    query_set = sample_df(args.sampling_map, args.query_set_size)

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

        cursor.execute(
            ";".join([
                f"SELECT {'COUNT(*)' if args.count_only else '*'} FROM query_by_cylinder_full_height("
                f"ST_SetSRID(ST_Point({longitude}, {latitude}),4326)::geography,"
                f"{altitude}::smallint,"
                f"{args.query_radius}"
                f")"
                for longitude, latitude, altitude in queries
            ])
        )

        # simulate fetching all results
        result_count_tmp = len(cursor.fetchall())

        # check whether we need to stop, if we have to do not count the last queries
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
@click.option(
    '--qps-set-size',
    '-q',
    'qps_set_size',
    type=int,
    default=5000
)
@click.option('--count-only', 'count_only', flag_value=True, default=False)
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
    batch_size: int,
    qps_set_size: int,
    count_only: bool
):

    sampling_map = load_sampling_map(sampling_map_path)

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
                    sampling_map=sampling_map,
                    db_host=db_host,
                    db_port=db_port,
                    db_name=db_name,
                    db_user=db_user,
                    db_pass=db_pass,
                    query_radius=query_radius,
                    batch_size=batch_size,
                    count_only=count_only,
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
    # start = time.time()
    start_event.set()
    # wait for the given time
    time.sleep(time_s)
    # then signal termination to the threads
    stop_event.set()

    # wait for all of them to terminate
    for process in processes:
        process.join()

    # stop = time.time()

    executed_queries = 0
    result_count = 0

    for xq, rc in zip(executed_queries_values, result_count_values):
        # read out result
        executed_queries += xq.value
        result_count += rc.value

    print(f"{num_threads},{time_s},{query_radius},{executed_queries},{result_count}")


if __name__ == '__main__':
    main()
