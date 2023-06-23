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

    # threads,time,include_certificates,successful_requests,failed_requests
    df = pd.read_csv(input_path)
    df['qps'] = df['successful_requests'] / df['time']
    df['fqps'] = df['failed_requests'] / df['time']

    df = df.groupby(
        by=['threads', 'include_certificates']
    ).agg(
        qps=('qps', 'mean'),
        qps_std=('qps', 'std'),
        fqps=('fqps', 'mean'),
        fqps_std=('fqps', 'std'),
    ).reset_index()

    fig, ax = plt.subplots(dpi=300)
    ax.set_xscale("log", base=2)
    ax.set_xlabel("number of parallel threads")
    ax.set_yscale("linear")
    ax.set_ylabel("queries per second")

    ax.errorbar(
        df['threads'],
        df['qps'],
        yerr=df['qps_std'],
        fmt='o',
        label=f"successful"
    )

    ax.errorbar(
        df['threads'],
        df['fqps'],
        yerr=df['fqps_std'],
        fmt='o',
        label=f"failed"
    )

    plt.legend(loc="upper left")
    plt.savefig(f"{output_path}/throughput.png")


if __name__ == '__main__':
    main()
