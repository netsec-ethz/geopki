from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import Polygon, Point
import click
import pandas as pd
import numpy as np
import subprocess
import os
import base64

from tqdm import tqdm

FILE_PATH = os.path.realpath(__file__)
QUERY_RADIUS = 10

# https://www.matecdev.com/posts/random-points-in-polygon.html


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
@click.argument('public_key', type=str)
@click.option(
    '--address',
    '-a',
    type=str,
    default='http://localhost:1234',
)
@click.option(
    '--locations',
    '-l',
    'location_count',
    type=int,
    default=1000
)
@click.option(
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    # by default do not repeat a single location
    default=1
)
def main(
    website_density_path: str,
    output_path: str,
    public_key: str,
    address: str,
    location_count: int,
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

    try:
        base64.b64decode(public_key)
    except:
        raise Exception("Invalid public key was passed as an argument")

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(f"longitude,latitude,radius,request_size,request_bit_string_count,response_size,response_node_count,certificate_hash_count,consistency_proof_size,time_building_query,time_send_receive,time_verification,time_consistency,time_total,include_certificates\n")
        f.flush()

    # probability 0 if osm_website_element_count == 0
    df['weight'] = df['osm_website_element_count']

    # very small probability if osm_website_element_count == 0
    # df['weights'] = df['osm_website_element_count'] + 1

    sample = df.sample(
        n=location_count,
        weights='weight',
        random_state=1,
        replace=True
    )

    # for each sample, sample a point within the polygon
    sample['sample_point'] = sample['polygon'].apply(sample_point_in_polygon)
    query_locations: list[
        tuple[float, float]
    ] = sample['sample_point'].values

    total_iterations = len(query_locations) * repetitions

    for i, longitude, latitude, include_certificates in tqdm(
        (
            (i, lon, lat, include_certificates)
            for (lon, lat) in query_locations
            for i in range(repetitions)
            for include_certificates in [False, True]
        ),
        total=total_iterations
    ):
        p = subprocess.Popen(
            [
                f"../../dist/geopki-client-performance",
                f"--address={address}",
                f"--public-key={public_key}",
                f"--longitude={longitude}",
                f"--latitude={latitude}",
                f"--radius={QUERY_RADIUS}",
            ]
            + (["--include-certificates"] if include_certificates else []),
            stdout=subprocess.PIPE,
            cwd=os.path.dirname(FILE_PATH)
        )

        f.write(p.stdout.read().decode("ascii"))
        f.flush()

    f.close()


if __name__ == '__main__':
    main()
