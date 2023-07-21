import click
import os
import pandas as pd
import matplotlib.pyplot as plt

FILE_PATH = os.path.realpath(__file__)


def cdf_plot(
    xlabel: str,
    df_column_excluding_certificates,
    df_column_including_certificates,
    output_filename: str,
    ax: plt.Axes = None,
    fig: plt.Figure = None,
    labels=["excluding certs", "including certs"],
    linestyles=["solid", "dashed"],
    base=10,
    plot=True,
    legend=False,
    show_quantile="circle",
    quantile_correction_factor=[1.2, 1.2],
    quantile_y=[0.934, 0.934]
) -> tuple[plt.Figure, plt.Axes]:
    if ax is None or fig is None:
        fig, ax = plt.subplots(dpi=300)
        fig.set_figheight(3)
        fig.set_figwidth(6)
        fig.subplots_adjust(bottom=0.16, left=0.09, right=0.99)
        # fig.set_figwidth(9)
        # fig.subplots_adjust(bottom=0.16, left=0.06, right=0.99)

        ax.set_yscale("linear")
        ax.set_ylabel("CDF")
        ax.set_xlabel(xlabel)
        if not base is None:
            ax.set_xscale("log", base=base)

    # https://stackoverflow.com/a/54317197
    s = pd.Series(df_column_excluding_certificates.values, name='value')
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
        label=labels[0],
        linestyle=linestyles[0]
    )
    color = p[0].get_color()

    ninety_five_row = stats_df[stats_df['cdf'] >= 0.95].iloc[0]
    ninety_five_p = ninety_five_row['cdf']
    ninety_five = ninety_five_row['value']

    print("-" * 80)
    print(xlabel)
    print(labels[0])
    print(f">= 95% of data points have a value <= {ninety_five}")

    if show_quantile == "circle":

        plt.scatter(
            [ninety_five],
            [ninety_five_p],
            marker="o",
            facecolors='none',
            edgecolors=color
        )

        plt.text(
            ninety_five * quantile_correction_factor[0],
            quantile_y[0],
            # f'({ninety_five}, {ninety_five_p.round(2)})',
            f'{int(ninety_five) if ninety_five.is_integer() else ninety_five.round(4)}',
            rotation=0,
            color=color
        )

    if not df_column_including_certificates is None:

        # fig, ax = plt.subplots(dpi=300)
        # ax.set_yscale("linear")
        # ax.set_ylabel("CDF")
        # ax.set_xlabel(xlabel)
        # ax.set_xscale("log", base=2)
        # including certs

        s = pd.Series(df_column_including_certificates.values, name='value')
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
            label=labels[1],
            linestyle=linestyles[1]
        )
        color = p[0].get_color()

        ninety_five_row = stats_df[stats_df['cdf'] >= 0.95].iloc[0]
        ninety_five_p = ninety_five_row['cdf']
        ninety_five = ninety_five_row['value']

        print(labels[1])
        print(f">= 95% of data points have a value <= {ninety_five}")

        if show_quantile == "circle":

            plt.scatter(
                [ninety_five],
                [ninety_five_p],
                marker="o",
                facecolors='none',
                edgecolors=color
            )

            plt.text(
                ninety_five * quantile_correction_factor[1],
                quantile_y[1],
                # f'({ninety_five}, {ninety_five_p.round(2)})',
                f'{int(ninety_five) if ninety_five.is_integer() else ninety_five.round(4)}',
                rotation=0,
                color=color
            )

    # left, right = ax.get_xlim()
    # bottom, top = ax.get_ylim()

    # # override the limits
    # bottom = 0

    # ax.set_xlim(left, right)
    # ax.set_ylim(bottom, top)

    if plot:
        if show_quantile == "line":
            left, right = plt.xlim()
            plt.xlim(left, right)
            plt.text(
                left * 1.1,
                0.97,
                f'0.95',
                rotation=0,
                color='silver'
            )
            plt.hlines(0.95, left, right, 'silver', linewidth=1)

        if isinstance(legend, str):
            plt.legend(loc=legend)

        plt.savefig(output_filename)
        plt.close()
    else:
        return fig, ax


def print_stats(df):
    df_mean_std = df.agg(
        request_size_max=('request_size', 'max'),
        request_size=('request_size', 'mean'),
        request_size_std=('request_size', 'std'),

        request_bit_string_count_max=('request_bit_string_count', 'max'),
        request_bit_string_count=('request_bit_string_count', 'mean'),
        request_bit_string_count_std=('request_bit_string_count', 'std'),

        response_size_max=('response_size', 'max'),
        response_size=('response_size', 'mean'),
        response_size_std=('response_size', 'std'),

        response_node_count_max=('response_node_count', 'max'),
        response_node_count=('response_node_count', 'mean'),
        response_node_count_std=('response_node_count', 'std'),

        certificate_hash_count_max=('certificate_hash_count', 'max'),
        certificate_hash_count_min=('certificate_hash_count', 'min'),
        certificate_hash_count=('certificate_hash_count', 'mean'),
        certificate_hash_count_std=('certificate_hash_count', 'std'),

        consistency_proof_size_max=('consistency_proof_size', 'max'),
        consistency_proof_size=('consistency_proof_size', 'mean'),
        consistency_proof_size_std=('consistency_proof_size', 'std'),

        time_building_query_max=('time_building_query', 'max'),
        time_building_query=('time_building_query', 'mean'),
        time_building_query_std=('time_building_query', 'std'),

        time_send_receive_max=('time_send_receive', 'max'),
        time_send_receive=('time_send_receive', 'mean'),
        time_send_receive_std=('time_send_receive', 'std'),

        time_verification_max=('time_verification', 'max'),
        time_verification=('time_verification', 'mean'),
        time_verification_std=('time_verification', 'std'),

        verification_fraction_max=('verification_fraction', 'max'),
        verification_fraction=('verification_fraction', 'mean'),
        verification_fraction_std=('verification_fraction', 'std'),

        query_build_fraction_max=('query_build_fraction', 'max'),
        query_build_fraction=('query_build_fraction', 'mean'),
        query_build_fraction_std=('query_build_fraction', 'std'),

        client_computation_fraction_max=('client_computation_fraction', 'max'),
        client_computation_fraction=('client_computation_fraction', 'mean'),
        client_computation_fraction_std=('client_computation_fraction', 'std'),

        time_consistency_max=('time_consistency', 'max'),
        time_consistency=('time_consistency', 'mean'),
        time_consistency_std=('time_consistency', 'std'),

        time_total_max=('time_total', 'max'),
        time_total=('time_total', 'mean'),
        time_total_std=('time_total', 'std'),
    ).agg(
        # collapse matrix
        "sum",
        axis="columns"
    )
    print(df_mean_std)


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

    df['verification_fraction'] = df['time_verification'] / df['time_total']
    df['query_build_fraction'] = df['time_building_query'] / df['time_total']
    df['client_computation_fraction'] = 1 - \
        (df['time_send_receive'] / df['time_total'])

    # longitude,latitude,radius,request_size,request_bit_string_count,response_size,response_node_count,certificate_hash_count,consistency_proof_size,time_building_query,time_send_receive,time_verification,time_consistency,time_total
    # print(df[df['response_node_count'] == 4173])
    # print(df[df['certificate_hash_count'] == 64])
    # exit()
    assert len(df[df['certificate_hash_count'] == 0]) == 0

    df_excluding_certificates = df[df['include_certificates'] == False]
    df_including_certificates = df[df['include_certificates'] == True]

    print("all")
    print_stats(df)
    print("excluding certificates")
    print_stats(df_excluding_certificates)
    print("including certificates")
    print_stats(df_including_certificates)

    # request_size
    fig, ax = cdf_plot(
        "request and response sizes in B",
        df['request_size'],
        None,
        f"{output_path}/request-size-cdf.png",
        base=2,
        labels=["request size", ""],
        plot=False,
        show_quantile=False,
    )

    ax.grid(axis='x', color='silver')

    # response_size
    cdf_plot(
        "response size in B",
        df_excluding_certificates['response_size'],
        df_including_certificates['response_size'],
        f"{output_path}/request-response-size-cdf.png",
        base=2,
        labels=[
            "response size excluding certs",
            "response size including certs"
        ],
        linestyles=["dashed", "dotted"],
        ax=ax,
        fig=fig,
        legend="lower right",
        show_quantile='line',
        quantile_correction_factor=[0.4, 1.15],
        quantile_y=[0.95, 0.92]
    )

    # request_bit_string_count
    cdf_plot(
        "request bit string count",
        df['request_bit_string_count'],
        None,
        f"{output_path}/request-bit-string-count-cdf.png",
        base=None,
        quantile_correction_factor=[0.995, 1],
        quantile_y=[0.87, 1]
    )

    # response_node_count
    cdf_plot(
        "response node count",
        df['response_node_count'],
        None,
        f"{output_path}/response-node-count-cdf.png",
        base=2,
        quantile_correction_factor=[0.93, 1],
        quantile_y=[0.83, 1]
    )

    # response hash count
    cdf_plot(
        "response certificate hash count",
        df['certificate_hash_count'],
        None,
        f"{output_path}/response-hash-count-cdf.png",
        base=None,
        quantile_correction_factor=[0.975, 1],
        quantile_y=[0.98, 1]
    )

    # consistency_proof_size
    # cdf_plot(
    #     "response consistency proof size",
    #     df_excluding_certificates['consistency_proof_size'],
    #     df_including_certificates['consistency_proof_size'],
    #     f"{output_path}/consistency-proof-size-cdf.png",
    # )

    # time_building_query
    fig, ax = cdf_plot(
        "query time in ms",
        df['time_building_query'] * 1000,
        None,
        f"{output_path}/time-build-query-cdf.png",
        plot=False,
        base=10,
        labels=["query build time", ""],
        linestyles=["solid", ""],
        show_quantile=False,
        # quantile_correction_factor=[1.15, 1],
        # quantile_y=[0.925, 1]
    )

    # time_send_receive
    # _, ax = cdf_plot(
    #     "request time in s",
    #     df_excluding_certificates['time_send_receive'],
    #     df_including_certificates['time_send_receive'],
    #     f"{output_path}/time-send-receive-cdf.png",
    #     plot=False,
    #     ax=ax,
    #     labels=[
    #         "request time exc. certs",
    #         "request time inc. certs"
    #     ],
    # )

    # time_verification
    cdf_plot(
        "query time in ms",
        df['time_verification'] * 1000,
        None,
        f"{output_path}/time-verification-cdf.png",
        fig=fig,
        ax=ax,
        plot=False,
        labels=[
            "verification time",
            ""
        ],
        linestyles=["dashed", ""],
        show_quantile=False,
        # quantile_correction_factor=[0.75, 0.45],
        # quantile_y=[1, 0.93]
    )

    # time_consistency
    # cdf_plot(
    #     "time to verify consistency in s",
    #     df_excluding_certificates['time_consistency'],
    #     df_including_certificates['time_consistency'],
    #     f"{output_path}/time-consistency-cdf.png",
    # )

    ax.grid(axis='x', color='silver')

    # time_total
    cdf_plot(
        "total request time in ms",
        df_excluding_certificates['time_total'] * 1000,
        df_including_certificates['time_total'] * 1000,
        f"{output_path}/time-total-cdf.png",
        ax=ax,
        fig=fig,
        legend="lower right",
        labels=[
            "total time exc. certs",
            "total time inc. certs"
        ],
        linestyles=["dotted", "dashdot"],
        show_quantile="line",
        # quantile_correction_factor=[0.55, 1.15],
        # quantile_y=[0.935, 0.935]
    )

    # query_build fraction
    # _, ax = cdf_plot(
    #     "fraction",
    #     df_excluding_certificates['query_build_fraction'],
    #     df_including_certificates['query_build_fraction'],
    #     f"{output_path}/query-build-fraction-cdf.png",
    #     labels=[
    #         "query build exc. certs",
    #         "query build inc. certs"
    #     ],
    #     plot=False,
    #     base=10,
    #     legend="best",
    #     linestyles=["solid", "dashed"],
    #     show_quantile=False,
    # )
    # cdf_plot(
    #     "fraction",
    #     df_excluding_certificates['verification_fraction'],
    #     df_including_certificates['verification_fraction'],
    #     f"{output_path}/verification-fraction-cdf.png",
    #     labels=[
    #         "verification exc. certs",
    #         "verification inc. certs"
    #     ],
    #     ax=ax,
    #     plot=False,
    #     base=None,
    #     legend="best",
    #     linestyles=["dotted", "dashdot"],
    #     show_quantile=False,
    # )
    cdf_plot(
        "client computation fraction",
        df_excluding_certificates['client_computation_fraction'],
        df_including_certificates['client_computation_fraction'],
        f"{output_path}/fraction-cdf.png",
        labels=[
            "excluding certs",
            "including certs"
        ],
        # ax=ax,
        # fig=fig,
        base=None,
        legend="best",
        # linestyles=[(0, (1, 10)), (0, (3, 5, 1, 5, 1, 5))],
        # show_quantile=False,
        quantile_correction_factor=[1.01, 0.85],
        quantile_y=[0.85, 0.95]
    )


if __name__ == '__main__':
    main()
