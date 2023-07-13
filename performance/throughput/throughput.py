from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import Polygon, Point
import click
import pandas as pd
import numpy as np
import subprocess
import os
import sys
import tempfile
import json
from tqdm import tqdm

sys.path.insert(1, os.path.join(sys.path[0], '../'))  # noqa - prevent auto formatting
from sampling import load_sampling_map, sample_df

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

QUERY_RADIUS = 10

TIME_VALUES = [8]
THREAD_VALUES = [1, 2, 4, 8, 16, 32, 64, 128]

MAX_QUERIES_PER_SECOND = 1000


class Query:
    def __init__(
            self,
            longitude: float,
            latitude: float,
            radius: int,
    ):
        self.longitude = longitude
        self.latitude = latitude
        self.radius = radius

    def to_json(self) -> dict[str, Any]:
        return {
            "longitude": self.longitude,
            "latitude": self.latitude,
            "radius": self.radius,
        }


@click.command()
@click.argument('sampling_map_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
@click.option(
    '--address',
    '-a',
    type=str,
    default='http://localhost:1234',
)
@click.option(
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    default=10
)
def main(
    sampling_map_path: str,
    output_path: str,
    address: str,
    repetitions: int,
):
    if os.path.isdir(output_path):
        raise Exception(f"Output path points to a directory")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(
            f"threads,time,include_certificates,successful_requests,failed_requests\n"
        )
        f.flush()

    df = load_sampling_map(sampling_map_path)

    total_iterations = len(TIME_VALUES) * len(THREAD_VALUES) * repetitions * 2

    for i, time, threads, include_certificates in tqdm(
        (
            (i, time, threads, include_certificates)
            for include_certificates in [False, True]
            for time in TIME_VALUES
            for threads in THREAD_VALUES
            for i in range(repetitions)
        ),
        total=total_iterations
    ):
        query_count = threads * time * MAX_QUERIES_PER_SECOND
        query_locations = sample_df(df, query_count)

        with tempfile.NamedTemporaryFile() as fp:

            # write queries to temporary file
            fp.write(
                json.dumps([
                    Query(longitude=longitude, latitude=latitude,
                          radius=QUERY_RADIUS).to_json()
                    for (longitude, latitude) in query_locations
                ]).encode("utf-8")
            )
            # flush to disk
            fp.flush()

            # print(f"go with {time}, {threads}, {fp.name}")
            p = subprocess.Popen(
                [
                    f"../../dist/geopki-client-throughput",
                    f"--address={address}",
                    f"--time={time}",
                    f"--threads={threads}",
                    f"--queries={fp.name}",
                ]
                + (["--include-certificates"] if include_certificates else []),
                stdout=subprocess.PIPE,
                cwd=CURRENT_DIR
            )

            f.write(p.stdout.read().decode("ascii"))
            f.flush()

    f.close()


if __name__ == '__main__':
    main()
