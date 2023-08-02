import click
import os
import numpy as np
import pandas as pd
import matplotlib.pyplot as plt
from ast import literal_eval
from itertools import chain

FILE_PATH = os.path.realpath(__file__)

# https://stackoverflow.com/a/39566040/2897827
MEDIUM_SIZE = 16
plt.rc('font', size=MEDIUM_SIZE)       # controls default text sizes
plt.rc('axes', titlesize=MEDIUM_SIZE)  # fontsize of the axes title
plt.rc('axes', labelsize=MEDIUM_SIZE)  # fontsize of the x and y labels
plt.rc('xtick', labelsize=MEDIUM_SIZE)  # fontsize of the tick labels
plt.rc('ytick', labelsize=MEDIUM_SIZE)  # fontsize of the tick labels
plt.rc('legend', fontsize=MEDIUM_SIZE)  # legend fontsize
plt.rc('figure', titlesize=MEDIUM_SIZE)  # fontsize of the figure title


def get_95_percentile(data):
    # https://stackoverflow.com/a/54317197
    s = pd.Series(data, name='value')
    df = pd.DataFrame(s)

    stats_df = df.groupby('value')['value'] \
        .agg('count') \
        .pipe(pd.DataFrame) \
        .rename(columns={'value': 'frequency'})

    # PDF
    stats_df['pdf'] = stats_df['frequency'] / sum(stats_df['frequency'])

    # CDF
    stats_df['cdf'] = stats_df['pdf'].cumsum()
    stats_df = stats_df.reset_index()

    ninety_five_row = stats_df[stats_df['cdf'] >= 0.95].iloc[0]
    # ninety_five_p = ninety_five_row['cdf']
    ninety_five = ninety_five_row['value']

    return ninety_five


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
    df = pd.read_csv(input_path, converters={"latencies": literal_eval})
    df['qps'] = df['successful_requests'] / df['time']
    df['fqps'] = df['failed_requests'] / df['time']

    df = df.groupby(
        by=['threads', 'include_certificates']
    ).agg(
        qps=('qps', 'mean'),
        qps_std=('qps', 'std'),
        fqps=('fqps', 'mean'),
        fqps_std=('fqps', 'std'),
        latencies=('latencies', lambda x: list(chain.from_iterable(x))),
    ).reset_index()

    df['latency_mean'] = df['latencies'].apply(np.mean)
    df['latency_median'] = df['latencies'].apply(np.median)
    df['latency_95'] = df['latencies'].apply(get_95_percentile)
    print(
        df[
            [
                'threads', 'include_certificates', 'qps',
                'latency_mean', 'latency_median', 'latency_95'
            ]
        ]
    )

    # plt.rcParams["figure.autolayout"] = True

    fig, ax = plt.subplots(dpi=300)
    fig.set_figheight(3)
    fig.set_figwidth(9)
    fig.subplots_adjust(top=0.99, bottom=0.2, left=0.09, right=0.995)

    ax.set_xscale("log", base=2)
    ax.set_xlabel("number of parallel goroutines")
    ax.set_yscale("linear")
    # ax.set_ylabel("queries per second")
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
