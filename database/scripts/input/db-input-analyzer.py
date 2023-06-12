import click
import pandas as pd
from tqdm import tqdm
from typing import Tuple, Optional
from shapely import Polygon
import matplotlib.pyplot as plt
import os
import sys

sys.path.insert(1, os.path.join(sys.path[0], '../../..'))  # noqa - prevent auto formatting
from coordinates import extruded_polygons_to_bit_string_counts
from coordinatez import DiscretizedVoxel

INITIAL_AREA_FRACTION = 1
VOXEL_PLOT_MAX = 1


def level_to_altitude(
    min_level: Optional[str],
    max_level: Optional[str],
    level: str
) -> Tuple[float, float]:
    if level == "@":
        # is a node
        # no floors in this building, just use the full height
        assert min_level == '@'
        assert max_level == '@'

        return DiscretizedVoxel.D, DiscretizedVoxel.H
    elif level == '':
        # is a building / area
        # if there are nodes within the building, min_level and max_level
        # might be set

        if min_level == '@' and max_level == '@':
            # all floors span the full building height
            # --> don't have any reference points
            return DiscretizedVoxel.D, DiscretizedVoxel.H
        elif min_level == '' and max_level == '':
            # some building without any areas / nodes within in
            return DiscretizedVoxel.D, DiscretizedVoxel.H
        else:
            # span the full height given by min_level and max_level
            min_level = float(min_level)
            max_level = float(max_level)

            # for now just assume one level is three meters
            # bound check in case of weirdly formatted data
            return max(min_level * 3, DiscretizedVoxel.D), min((max_level + 1) * 3, DiscretizedVoxel.H)
    else:
        # use float(), apparently there is floor -0.5 in the dataset
        try:
            level = float(level)
            min_level = float(min_level)
            max_level = float(max_level)

            assert min_level <= level and level <= max_level

            # for now just assume one level is three meters
            # bound check in case of weirdly formatted data
            return (
                min(
                    max(min_level * 3, DiscretizedVoxel.D),
                    DiscretizedVoxel.H
                ),
                min(
                    max((max_level + 1) * 3, DiscretizedVoxel.D),
                    DiscretizedVoxel.H
                )
            )

        except ValueError:
            print(level, min_level, max_level)
            raise


@click.command()
@click.argument('input_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=True))
def main(
    input_path: str,
    output_path: str
):

    if input_path.endswith(".parquet"):
        df = pd.read_parquet(input_path)
    elif input_path.endswith(".csv"):
        df = pd.read_csv(input_path)
    else:
        raise Exception(f"Unkown input file type")

    if not os.path.isdir(output_path):
        raise Exception(f"Output path has to be a directory")

    bit_string_counts = []
    xy_bit_string_counts = []

    for (idx, row) in tqdm(
        df.iterrows(),
        total=len(df),
        desc="locate buildings"
    ):

        domain = row['domain']

        if domain is None or not isinstance(domain, str) or domain.strip() == '':
            continue

        list_of_multipolygons = row['list_of_multipolygons']
        list_of_levels = row['list_of_levels']
        min_level = row['min_building_level']
        max_level = row['max_building_level']

        assert len(list_of_multipolygons) == len(list_of_levels)

        for i, (multipolygon, level) in enumerate(zip(list_of_multipolygons, list_of_levels)):

            try:
                altitude_min, altitude_max = level_to_altitude(
                    min_level,
                    max_level,
                    level
                )
            except (ValueError, AssertionError):
                print(row)
                raise

            shapely_polygons = [
                Polygon(
                    [(x['lon'], x['lat']) for x in polygon]
                )
                for polygon in multipolygon
            ]

            c, d = extruded_polygons_to_bit_string_counts(
                polygons=shapely_polygons,
                altitude_min=altitude_min,
                altitude_max=altitude_max,
                f_grow=INITIAL_AREA_FRACTION
            )

            bit_string_counts.append(c)
            xy_bit_string_counts.append(d)

    bit_string_counts_df = pd.Series(
        bit_string_counts, dtype=int
    ).value_counts().reset_index()
    bit_string_counts_df.columns = ['value', 'count']

    bit_string_counts_df.sort_values(
        by=['value'],
        ascending=True,
        inplace=True
    )

    xy_bit_string_counts_df = pd.Series(
        xy_bit_string_counts, dtype=int
    ).value_counts().reset_index()
    xy_bit_string_counts_df.columns = ['value', 'count']

    xy_bit_string_counts_df.sort_values(
        by=['value'],
        ascending=True,
        inplace=True
    )

    fig, ax = plt.subplots(dpi=300)
    ax.scatter(
        bit_string_counts_df['value'],
        bit_string_counts_df['count'],
        label="3d bit strings"
    )
    ax.scatter(
        xy_bit_string_counts_df['value'],
        xy_bit_string_counts_df['count'],
        label="2d bit strings"
    )
    ax.set_ylabel("# of polygons")
    ax.set_xlabel("# of bit strings")
    plt.legend(loc="upper right")

    fig.savefig(
        f"{output_path}/frequency.png"
    )


if __name__ == '__main__':
    main()
