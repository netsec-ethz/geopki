from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import Polygon, Point
import numpy as np
import pandas as pd
import click
from getpass import getpass
import subprocess
import os

from tqdm import tqdm

FILE_PATH = os.path.realpath(__file__)

CENTER_VALUES: list[tuple[float, float, float]] = [
    (8.5470994, 47.3762348, 0),
]
F_GROW_VALUES: list[float] = [
    1,
    0.5,
    0.25,
    0.125,
    0.0625,
    0
]
QUERY_RADIUS_VALUES: list[int] = [5, 10, 20, 40]


def sample_point_in_polygon(polygon: Polygon) -> tuple[float, float]:
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
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    default=100
)
def main(
    website_density_path: str,
    output_path: str,
    repetitions: int,
):

    if os.path.isdir(output_path):
        raise Exception(f"Output path has to point to a file")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

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

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(f"f,radius,time,bitstring_count\n")
        f.flush()

    # probability 0 if osm_website_element_count == 0
    df['weight'] = df['osm_website_element_count']

    # very small probability if osm_website_element_count == 0
    # df['weights'] = df['osm_website_element_count'] + 1

    sample = df.sample(
        n=repetitions,
        weights='weight',
        random_state=1,
        replace=True
    )

    # for each sample, sample a point within the polygon
    sample['sample_point'] = sample['polygon'].apply(sample_point_in_polygon)
    query_locations: list[
        tuple[float, float]
    ] = sample['sample_point'].values

    total_iterations = len(query_locations) * len(F_GROW_VALUES) * \
        len(QUERY_RADIUS_VALUES)

    for longitude, latitude, altitude, radius, fGrow in tqdm(
        (
            # use 0 for the altitude
            (lon, lat, 0, r, f)
            for r in QUERY_RADIUS_VALUES
            for f in F_GROW_VALUES
            for (lon, lat) in query_locations
        ),
        total=total_iterations
    ):
        p = subprocess.Popen(
            [
                f"go",
                f"run",
                f"../../cmd/bitstring-performance",
                f"--longitude={longitude}",
                f"--latitude={latitude}",
                f"--altitude={altitude}",
                f"--radius={radius}",
                f"--f={fGrow}",
                f"--repetitions={1}",
            ],
            stdout=subprocess.PIPE,
            cwd=os.path.dirname(FILE_PATH)
        )

        f.write(p.stdout.read().decode("ascii"))
        f.flush()

    f.close()


if __name__ == '__main__':
    main()
