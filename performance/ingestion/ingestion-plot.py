import click
import os
import pandas as pd
import matplotlib.pyplot as plt

FILE_PATH = os.path.realpath(__file__)


def cdf_plot(
    label: str,
    ax: plt.Axes,
    df_column,
    linestyle: str,
    show_quantile=True,
    quantile_correction_factor=1.01,
    quantile_y=0.92
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

    p = plt.plot(
        stats_df['value'],
        stats_df['cdf'],
        label=label,
        linestyle=linestyle
    )
    color = p[0].get_color()

    if show_quantile:
        ninety_five_row = stats_df[stats_df['cdf'] >= 0.95].iloc[0]
        ninety_five_p = ninety_five_row['cdf']
        ninety_five = ninety_five_row['value']

        plt.scatter(
            [ninety_five],
            [ninety_five_p],
            marker="o",
            facecolors='none',
            edgecolors=color
        )

        plt.text(
            ninety_five * quantile_correction_factor,
            quantile_y,
            # f'({ninety_five}, {ninety_five_p.round(2)})',
            f'{int(ninety_five) if ninety_five.is_integer() else ninety_five.round(4)}',
            rotation=0,
            color=color
        )


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
    for certificate_count, linestyle in zip(
        certificate_counts,
        ["solid", "dotted", "dashed", "dashdot"]
    ):
        x = df[df['certificate_count'] == certificate_count]
        cdf_plot(
            f"batch of {certificate_count} per request",
            ax,
            x['spc'],
            linestyle
        )

    plt.legend()
    plt.savefig(f"{output_path}/ingestion.png")


if __name__ == '__main__':
    main()
