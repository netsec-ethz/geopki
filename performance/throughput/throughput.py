from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import Polygon, Point
import click
import pandas as pd
import numpy as np
import subprocess
import os
import tempfile
import json

from tqdm import tqdm

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

QUERY_RADIUS = 10

TIME_VALUES = [8]
THREAD_VALUES = [1, 2, 4, 8, 16, 32, 64, 128]

MAX_QUERIES_PER_SECOND = 50


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


def sample_point_in_polygon(polygon: Polygon) -> tuple[float, float]:
    "https://www.matecdev.com/posts/random-points-in-polygon.html"
    minX, minY, maxX, maxY = polygon.bounds

    while True:
        # rejection sampling
        sample = Point(np.random.uniform(minX, maxX),
                       np.random.uniform(minY, maxY))
        if polygon.contains(sample):
            return sample.x, sample.y


@click.command()
@click.argument('website_density_path', type=click.Path(exists=True))
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
    website_density_path: str,
    output_path: str,
    address: str,
    repetitions: int,
):

    if not os.path.isfile(website_density_path):
        raise Exception(f"Website density path does to point to a file")

    if not website_density_path.endswith(".parquet"):
        raise Exception(f"Website density path does to end in '.parquet'")

    df = pd.read_parquet(website_density_path)

    df['polygon'] = df['polygon'].apply(
        lambda points:
        Polygon([
            (p[1], p[0])
            for p in points
        ])
    )

    if os.path.isdir(output_path):
        raise Exception(f"Output path points to a directory")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(
            f"threads,time,include_certificates,successful_requests,failed_requests,include_certificates\n"
        )
        f.flush()

    # probability 0 if osm_website_element_count == 0
    df['weight'] = df['osm_website_element_count']

    # very small probability if osm_website_element_count == 0
    # df['weights'] = df['osm_website_element_count'] + 1

    total_iterations = len(TIME_VALUES) * len(THREAD_VALUES) * repetitions

    for i, time, threads, include_certificates in tqdm(
        (
            (i, time, threads, include_certificates)
            for time in TIME_VALUES
            for threads in THREAD_VALUES
            for include_certificates in [False, True]
            for i in range(repetitions)
        ),
        total=total_iterations
    ):
        query_count = threads * time * MAX_QUERIES_PER_SECOND

        sample = df.sample(
            n=query_count,
            weights='weight',
            random_state=1,
            replace=True
        )

        # for each sample, sample a point within the polygon
        sample['sample_point'] = sample['polygon'].apply(
            sample_point_in_polygon)
        query_locations: list[
            tuple[float, float]
        ] = sample['sample_point'].values

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
