from typing import Dict, Any, Union, Optional, List, Tuple
import click
from getpass import getpass
import subprocess
import os

from tqdm import tqdm

FILE_PATH = os.path.realpath(__file__)

THREAD_VALUES = [1, 2, 4, 8, 16, 32, 64, 128]
TIME_VALUES = [8]
QUERY_RADIUS_VALUES = [10]


@click.command()
@click.argument('sampling_map_path', type=click.Path(exists=False))
@click.argument('output_path', type=click.Path(exists=False))
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
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    default=10
)
@click.option('--postgres-bitstrings', 'mode', flag_value='postgres_bitstrings', default=None)
@click.option('--postgres-bitstrings-int', 'mode', flag_value='postgres_bitstrings_int', default=None)
@click.option('--postgres-bitstrings-int-z', 'mode', flag_value='postgres_bitstrings_int_z', default=None)
@click.option('--postgres-spatial', 'mode', flag_value='postgres_spatial', default=None)
@click.option('--postgres-baseline', 'mode', flag_value="postgres_baseline")
@click.option('--neo4j', 'mode', flag_value='neo4j')
@click.option('--excluding-bit-string-computation', 'excluding_bit_string_computation', flag_value=True, default=False)
@click.option('--count-only', 'count_only', flag_value=True, default=False)
@click.option('--batch-size', '-b', 'batch_size', type=int, default=100)
@click.option('--qps-set-size', '-q', 'qps_set_size', type=int, default=1000)
def main(
    sampling_map_path: str,
    output_path: str,
    db_host: str,
    db_port: int,
    db_name: str,
    db_user: str,
    db_pass: Optional[str],
    repetitions: int,
    mode: Optional[str],
    excluding_bit_string_computation: bool,
    count_only: bool,
    batch_size: Optional[int],
    qps_set_size: int,
):

    if not os.path.isfile(sampling_map_path):
        raise Exception(f"sampling map path does not point to a file")

    if os.path.isdir(output_path):
        raise Exception(f"Output path has to point to a file")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

    if db_pass is None:
        db_pass = getpass(
            f"Password for {db_user}:{db_name}@{db_host}:{db_port}: "
        )

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(f"num_threads,time_s,query_radius_m,executed_queries,result_count\n")
        f.flush()

    total_iterations = len(THREAD_VALUES) * len(TIME_VALUES) * \
        len(QUERY_RADIUS_VALUES) * repetitions

    executable: Optional[str] = None
    if mode == "postgres_baseline":
        executable = "./db-types/postgres-baseline-run.py"
    elif mode == "postgres_spatial":
        executable = "./db-types/postgres-spatial-run.py"
    elif mode == "postgres_bitstrings":
        executable = "./db-types/postgres-bitstring-run.py"
    elif mode == "postgres_bitstrings_int":
        executable = "./db-types/postgres-bitstring-int-run.py"
    elif mode == "postgres_bitstrings_int_z":
        executable = "./db-types/postgres-bitstring-int-z-subtrees-run.py"
    elif mode == "neo4j":
        executable = "./db-types/neo4j-run.py"
    else:
        raise Exception(f"Unkown mode '{mode}'")

    for num_threads, time_s, query_radius_m, repetition_idx in tqdm(
        (
            (n, t, r, i)
            for n in THREAD_VALUES
            for t in TIME_VALUES
            for r in QUERY_RADIUS_VALUES
            for i in range(repetitions)
        ),
        total=total_iterations
    ):
        p = subprocess.Popen(
            [
                f"python3",
                executable,
                f"--sampling-map={sampling_map_path}",
                f"--db-host={db_host}",
                f"--db-port={db_port}",
                f"--db-name={db_name}",
                f"--db-user={db_user}",
                f"--db-pass={db_pass}",
                f"--threads={num_threads}",
                f"--time={time_s}",
                f"--query-radius={query_radius_m}",
            ]
            + (
                [f"--qps-set-size={qps_set_size}"]
                if mode != "postgres_bitstrings_int_z" else []
            )
            + (
                [f"--excluding-bit-string-computation"]
                if excluding_bit_string_computation else []
            )
            + (
                [f"--count-only"]
                if count_only else []
            )
            + (
                [f"--batch-size={batch_size}"]
                if not batch_size is None else []
            ),
            stdout=subprocess.PIPE,
            cwd=os.path.dirname(FILE_PATH)
        )

        f.write(p.stdout.read().decode("ascii"))
        f.flush()

    f.close()


if __name__ == '__main__':
    main()
