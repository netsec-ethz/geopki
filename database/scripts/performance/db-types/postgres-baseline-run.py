from typing import Dict, Any, Union, Optional, List, Tuple
import click
from getpass import getpass
import time
import random
from multiprocessing import Pool, Process, Event, Value

import psycopg2


class ProcessArgs:
    def __init__(
            self,
            db_host: str,
            db_port: int,
            db_name: str,
            db_user: str,
            db_pass: str,
            query_radius: int,
            batch_size: int,
            start_event: Event,
            stop_event: Event
    ) -> None:
        self.db_host = db_host
        self.db_port = db_port
        self.db_name = db_name
        self.db_user = db_user
        self.db_pass = db_pass
        self.query_radius = query_radius
        self.batch_size = batch_size
        self.start_event = start_event
        self.stop_event = stop_event


def run_queries(args: ProcessArgs, executed_queries_value: Value, result_count_value: Value):
    result_count = 0
    executed_queries = 0

    conn = psycopg2.connect(
        host=args.db_host,
        port=args.db_port,
        database=args.db_name,
        user=args.db_user,
        password=args.db_pass
    )

    cursor = conn.cursor()

    # wait for start signal
    args.start_event.wait()

    # work until stop is signalled
    while True:
        # ≈ 25 million entries, each has 50% of existing
        xs = [random.randint(0, 50000000) for _ in range(args.batch_size)]

        # execute query / queries

        cursor.execute(
            ";".join([
                f"SELECT id, payload "
                f"FROM sanity_check "
                f"WHERE id = {x}"
                for x in xs
            ])
        )

        # simulate fetching all results
        result_count_temp = len(cursor.fetchall())

        # check whether we need to stop
        if args.stop_event.is_set():
            break

        # if not stopped yet, add results
        result_count += result_count_temp

        # increase query counter by one
        executed_queries += args.batch_size

    cursor.close()
    conn.close()

    executed_queries_value.value = executed_queries
    result_count_value.value = result_count


@click.command()
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
    '--batch-size',
    '-b',
    'batch_size',
    type=int,
    default=100
)
def main(
    db_host: str,
    db_port: int,
    db_name: str,
    db_user: str,
    db_pass: Optional[str],
    num_threads: int,
    time_s: int,
    query_radius: int,
    batch_size
):

    if db_pass is None:
        db_pass = getpass(
            f"Password for {db_user}:{db_name}@{db_host}:{db_port}: "
        )

    start_event = Event()
    stop_event = Event()

    executed_queries_values = [
        Value("i", 0, lock=False)
        for _ in range(num_threads)
    ]
    result_count_values = [
        Value("i", 0, lock=False)
        for _ in range(num_threads)
    ]

    args = ProcessArgs(
        db_host=db_host,
        db_port=db_port,
        db_name=db_name,
        db_user=db_user,
        db_pass=db_pass,
        query_radius=query_radius,
        batch_size=batch_size,
        start_event=start_event,
        stop_event=stop_event
    )

    processes: List[Process] = [
        Process(
            target=run_queries,
            args=(args, executed_queries_values, result_count_value)
        )
        for executed_queries_values, result_count_value
        in zip(executed_queries_values, result_count_values)
    ]

    # initiate all threads
    for process in processes:
        process.start()

    # give start signal
    start_event.set()
    # wait for the given time
    time.sleep(time_s)
    # then signal termination to the threads
    stop_event.set()

    for process in processes:
        process.join()

    executed_queries = 0
    result_count = 0

    # wait for all of them to terminate
    for xq, rc in zip(executed_queries_values, result_count_values):
        # read out result
        executed_queries += xq.value
        result_count += rc.value

    print(f"{num_threads},{time_s},{query_radius},{executed_queries},{result_count}")


if __name__ == '__main__':
    main()
