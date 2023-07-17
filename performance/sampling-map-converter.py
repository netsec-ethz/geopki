from typing import Dict, Any, Union, Optional, List, Tuple
import click
import pandas as pd
import numpy as np
import os
import sys
import json
from tqdm import tqdm

sys.path.insert(1, os.path.join(sys.path[0], '../'))  # noqa - prevent auto formatting
from sampling import load_bit_string_sampling_map, sample_df

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

QUERY_RADIUS = 10


class NpEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, np.integer):
            return int(obj)
        if isinstance(obj, np.floating):
            return float(obj)
        if isinstance(obj, np.ndarray):
            return obj.tolist()
        return super(NpEncoder, self).default(obj)


class Query:
    def __init__(
            self,
            bit_strings: list[str],
    ):

        self.bit_strings = bit_strings

    def to_json(self) -> dict[str, Any]:
        return {
            "bit_strings": self.bit_strings,
        }


@click.command()
@click.argument('sampling_map_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
def main(
    sampling_map_path: str,
    output_path: str,
):
    if os.path.isdir(output_path):
        raise Exception(f"Output path points to a directory")

    if not output_path.endswith(".json"):
        raise Exception(f"Output path extension has to be .json")

    query_locations = load_bit_string_sampling_map(sampling_map_path).values

    with open(output_path, 'w') as f:
        f.write(
            json.dumps([
                Query(
                    bit_strings=bit_strings[0],
                ).to_json()

                for bit_strings in tqdm(
                    query_locations,
                    total=len(query_locations)
                )
            ],
                cls=NpEncoder
            )
        )


if __name__ == '__main__':
    main()
