from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import Polygon, Point
import click
import pandas as pd
import numpy as np
import subprocess
import os
import sys
import base64
from tqdm import tqdm

sys.path.insert(1, os.path.join(sys.path[0], '../'))  # noqa - prevent auto formatting
from sampling import sample

FILE_PATH = os.path.realpath(__file__)
QUERY_RADIUS = 10


@click.command()
@click.argument('sampling_map_path', type=click.Path(exists=True))
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
def main(
    sampling_map_path: str,
    output_path: str,
    public_key: str,
    address: str,
    location_count: int,
):
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

    query_locations = sample(sampling_map_path, location_count)

    for i, (longitude, latitude) in tqdm(
        enumerate(query_locations),
        total=location_count
    ):
        include_certificates = (i >= (location_count / 2))

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
