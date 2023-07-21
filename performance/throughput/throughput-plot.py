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

    # plt.rcParams["figure.autolayout"] = True

    fig, ax = plt.subplots(dpi=300)
    fig.set_figheight(3)
    fig.set_figwidth(6)
    fig.subplots_adjust(bottom=0.16, left=0.13, right=0.99)
    # fig.set_figwidth(9)
    # fig.subplots_adjust(bottom=0.16, left=0.09, right=0.99)
    ax.set_xscale("log", base=2)
    ax.set_xlabel("number of parallel goroutines")
    ax.set_yscale("linear")
    ax.set_ylabel("queries per second")
    # ax.set_yticks([])

    ax.grid(axis='y', color='silver')

    df_excluding_certificates = df[df['include_certificates'] == False]
    df_including_certificates = df[df['include_certificates'] == True]

    ax.errorbar(
        df_excluding_certificates['threads'],
        df_excluding_certificates['qps'],
        yerr=df_excluding_certificates['qps_std'],
        fmt='x',
        fillstyle='none',
        label=f"excluding certificates"
    )

    if len(df_excluding_certificates[df_excluding_certificates['fqps'] > 0]) > 0:
        ax.errorbar(
            df_excluding_certificates['threads'],
            df_excluding_certificates['fqps'],
            yerr=df_excluding_certificates['fqps_std'],
            fmt='+',
            fillstyle='none',
            label=f"failed excluding certificates"
        )

    if len(df_including_certificates) > 0:
        ax.errorbar(
            df_including_certificates['threads'],
            df_including_certificates['qps'],
            yerr=df_including_certificates['qps_std'],
            fmt='h',
            fillstyle='none',
            label=f"including certificates"
        )

        if len(df_including_certificates[df_including_certificates['fqps'] > 0]) > 0:
            ax.errorbar(
                df_including_certificates['threads'],
                df_including_certificates['fqps'],
                yerr=df_including_certificates['fqps_std'],
                fmt='d',
                fillstyle='none',
                label=f"failed including certificates"
            )

    plt.legend(loc="upper left")
    # fig.tight_layout()
    # plt.show()
    plt.savefig(f"{output_path}/throughput.png")


if __name__ == '__main__':
    main()
