import click
import os
import pandas as pd
import matplotlib.pyplot as plt

FILE_PATH = os.path.realpath(__file__)


def cdf_plot(
    label: str,
    ax: plt.Axes,
    df_column,
    show_quantile=True,
):

    # https://stackoverflow.com/a/54317197
    s = pd.Series(df_column.values, name='value')
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
    cdf = stats_df['cdf'].values

    plt.plot(stats_df['value'], stats_df['cdf'], label=label)

    left, right = ax.get_xlim()
    bottom, top = ax.get_ylim()

    # override the limits
    bottom = 0

    # ax.set_xlim(left, right)
    # ax.set_ylim(bottom, top)


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

    # certificate_count,time,success

    df['spc'] = df['time'] / df['certificate_count']

    fig, ax = plt.subplots(dpi=300)
    ax.set_yscale("linear")
    ax.set_ylabel("CDF")
    ax.set_xlabel("mean time per certificate in s")

    certificate_counts = df['certificate_count'].unique()
    for certificate_count in certificate_counts:
        x = df[df['certificate_count'] == certificate_count]
        cdf_plot(f"batch of {certificate_count}", ax, x['spc'])

    plt.legend()
    plt.savefig(f"{output_path}/cdf.png")


if __name__ == '__main__':
    main()
