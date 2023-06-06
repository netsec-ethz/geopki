import click
import os
import pandas as pd
import matplotlib.pyplot as plt

FILE_PATH = os.path.realpath(__file__)


@click.command()
@click.argument('input_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
def main(
    input_path: str,
    output_path: str
):

    if os.path.isdir(input_path):
        raise Exception(f"Input path has to point to a file")

    if not input_path.endswith(".csv"):
        raise Exception(f"Input path extension has to be .csv")

    if not os.path.isdir(output_path):
        raise Exception(f"Output path has to point to a directory")

    df = pd.read_csv(input_path)
    df = df.groupby(
        by=['f', 'radius']
    ).agg(
        time=('time', 'mean'),
        time_std=('time', 'std'),
        count=('bitstring_count', 'mean'),
        count_std=('bitstring_count', 'std'),
    ).reset_index()

    fig, ax = plt.subplots(dpi=300)
    ax.set_xscale("linear")
    ax.set_xlabel("$f$")
    ax.set_yscale("log")
    ax.set_ylabel("time in seconds")

    for r in df['radius'].unique():
        x = df[df['radius'] == r]

        ax.errorbar(
            x['f'],
            x['time'],
            yerr=x['time_std'],
            fmt='o',
            label=f"radius: {r}m"
        )

    plt.legend(loc="upper right")
    plt.savefig(f"{output_path}/f-time.png")

    fig, ax = plt.subplots(dpi=300)
    ax.set_xscale("linear")
    ax.set_xlabel("$f$")
    ax.set_yscale("log")
    ax.set_ylabel("xy bit string count")

    for r in df['radius'].unique():
        x = df[df['radius'] == r]

        ax.errorbar(
            x['f'],
            x['count'],
            yerr=x['count_std'],
            fmt='o',
            label=f"radius: {r}m"
        )

    plt.legend(loc="upper right")
    plt.savefig(f"{output_path}/f-count.png")


if __name__ == '__main__':
    main()
