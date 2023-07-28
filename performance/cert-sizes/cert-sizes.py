import click
import os
import pandas as pd
import matplotlib.pyplot as plt

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


def cdf_plot(
    label: str,
    ax: plt.Axes,
    df_column,
    linestyle: str,
    show_quantile=True,
    quantile=0.95,
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
        ninety_five_row = stats_df[stats_df['cdf'] >= quantile].iloc[0]
        ninety_five_p = ninety_five_row['cdf']
        ninety_five = ninety_five_row['value']

        plt.scatter(
            [ninety_five],
            [ninety_five_p],
            marker="o",
            s=100,
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
    print(f"total of {len(df)} certificates")
    print(f"mean {df['length'].mean()}")
    print(f"std: {df['length'].std()}")
    print(f"median: {df['length'].median()}")

    fig, ax = plt.subplots(dpi=300)
    fig.set_figheight(3)
    fig.set_figwidth(9)
    fig.subplots_adjust(top=0.99, bottom=0.2, left=0.07, right=0.995)

    ax.set_yscale("linear")
    # ax.set_ylabel("CDF")
    ax.set_xlabel("certificate size in B")

    cdf_plot(
        "all",
        ax,
        df['length'],
        "solid",
        quantile=0.99,
        quantile_correction_factor=40,
        quantile_y=0.88
    )

    plt.savefig(f"{output_path}/cert-sizes-cdf.png")

    fig, ax = plt.subplots(dpi=300)
    fig.set_figheight(3)
    fig.set_figwidth(9)
    fig.subplots_adjust(top=0.99, bottom=0.2, left=0.07, right=0.995)

    ax.set_yscale("linear")
    # ax.set_ylabel("CDF")
    ax.set_xlabel("certificate size in B")

    df_reduced = df[df['length'] <= 3091]

    cdf_plot(
        r"size $\geq$ 3091",
        ax,
        df_reduced['length'],
        "solid",
        quantile=0.99,
        quantile_correction_factor=0.96,
        quantile_y=0.85
    )

    # plt.legend()
    plt.savefig(f"{output_path}/cert-sizes-reduced-cdf.png")


if __name__ == '__main__':
    main()
