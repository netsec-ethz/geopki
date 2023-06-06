from typing import Dict, Any, Union, Optional, List, Tuple
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


@click.command()
@click.argument('output_path', type=click.Path(exists=False))
@click.option(
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    default=10
)
def main(
    output_path: str,
    repetitions: int,
):

    if os.path.isdir(output_path):
        raise Exception(f"Output path has to point to a file")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(f"f,radius,time,bitstring_count\n")
        f.flush()

    total_iterations = len(CENTER_VALUES) * len(F_GROW_VALUES) * \
        len(QUERY_RADIUS_VALUES)

    for longitude, latitude, altitude, radius, fGrow in tqdm(
        (
            (lon, lat, alt, r, f)
            for (lon, lat, alt) in CENTER_VALUES
            for r in QUERY_RADIUS_VALUES
            for f in F_GROW_VALUES
        ),
        total=total_iterations
    ):
        p = subprocess.Popen(
            [
                f"go",
                f"run",
                f"./cmd/bitstring-performance",
                f"--longitude={longitude}",
                f"--latitude={latitude}",
                f"--altitude={altitude}",
                f"--radius={radius}",
                f"--f={fGrow}",
                f"--repetitions={repetitions}",
            ],
            stdout=subprocess.PIPE,
            cwd=os.path.dirname(FILE_PATH)
        )

        f.write(p.stdout.read().decode("ascii"))
        f.flush()

    f.close()


if __name__ == '__main__':
    main()
