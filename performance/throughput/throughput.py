import click
import subprocess
import os
import sys
from tqdm import tqdm

sys.path.insert(1, os.path.join(sys.path[0], '../'))  # noqa - prevent auto formatting

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

TIME_VALUES = [8]
THREAD_VALUES = [1, 2, 4, 8, 16, 32, 64, 128]


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
        # print(f"go with {time}, {threads}, {fp.name}")
        p = subprocess.Popen(
            [
                f"../../dist/geopki-client-throughput",
                f"--address={address}",
                f"--time={time}",
                f"--threads={threads}",
                f"--queries={sampling_map_path}",
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
