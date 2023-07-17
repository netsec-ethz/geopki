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
sys.path.insert(1, os.path.join(sys.path[0], '../../'))  # noqa - prevent auto formatting
from coordinatez import GeodeticCoordinate, polygons_to_2d_bit_strings, sphere_to_polygon

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

QUERY_RADIUS = 10
F_GROW = 1


def row_to_query(query: Tuple[float, float], query_radius: float) -> dict[str, Any]:
    longitude, latitude = query

    bit_strings = polygons_to_2d_bit_strings(
        polygons=[sphere_to_polygon(
            center=GeodeticCoordinate(
                longitude=longitude,
                latitude=latitude,
                altitude=0
            ),
            radius_m=query_radius
        )],
        f_grow=F_GROW,
        f_min=0
    )

    return {
        'bit_strings': bit_strings,
    }


@click.command()
@click.argument('sampling_map_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
@click.argument('f_grow', type=float, default=1)
def main(
    sampling_map_path: str,
    output_path: str,
    f_grow: float,
):
    global F_GROW
    F_GROW = f_grow

    if os.path.isdir(output_path):
        raise Exception(f"Output path points to a directory")

    if not output_path.endswith(".parquet"):
        raise Exception(f"Output path extension has to be .parquet")

    rows: list[tuple[list[str], int, int]] = []
    locations = load_sampling_map(sampling_map_path).values

    for i, (longitude, latitude) in tqdm(
        (
            enumerate(locations)
        ),
        total=len(locations)
    ):
        rows.append(row_to_query((longitude, latitude), QUERY_RADIUS))

    df = pd.DataFrame.from_dict(rows)
    df.to_parquet(output_path, index=False)


if __name__ == '__main__':
    main()
