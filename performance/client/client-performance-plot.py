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
    labels=["excluding certs", "including certs"],
    linestyles=["solid", "dashed"],
    base=10,
    plot=True,
    show_quantile=True,
):
    if ax is None:
        fig, ax = plt.subplots(dpi=300)
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

    plt.plot(
        stats_df['value'],
        stats_df['cdf'],
        label=labels[0],
        linestyle=linestyles[0]
    )
    # plt.savefig(output_filename)
    # plt.close()

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

        plt.plot(
            stats_df['value'],
            stats_df['cdf'],
            label=labels[1],
            linestyle=linestyles[1]
        )
        plt.legend()

    if plot:
        plt.savefig(output_filename)
        plt.close()
    else:
        return ax

    # left, right = ax.get_xlim()
    # bottom, top = ax.get_ylim()

    # # override the limits
    # bottom = 0

    # ax.set_xlim(left, right)
    # ax.set_ylim(bottom, top)

    # if show_quantile:
    #     ninety_five_row = stats_df[stats_df['cdf'] >= 0.95].iloc[0]
    #     ninety_five_p = ninety_five_row['cdf']
    #     ninety_five = ninety_five_row['value']

    #     # print(nin)

    #     ax.hlines(y=ninety_five_p, xmin=left,
    #               xmax=ninety_five, linewidth=0.5, color='r')
    #     ax.vlines(x=ninety_five, ymin=bottom,
    #               ymax=ninety_five_p, linewidth=0.5, color='r')

    #     plt.text(
    #         ninety_five * 1.05,
    #         0.92,
    #         f'({ninety_five}, {ninety_five_p.round(2)})',
    #         rotation=0,
    #         color='r'
    #     )
    # plt.savefig(output_filename)


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

    # longitude,latitude,radius,request_size,request_bit_string_count,response_size,response_node_count,certificate_hash_count,consistency_proof_size,time_building_query,time_send_receive,time_verification,time_consistency,time_total

    df_mean_std = df.agg(
        request_size=('request_size', 'mean'),
        request_size_std=('request_size', 'std'),

        request_bit_string_count=('request_bit_string_count', 'mean'),
        request_bit_string_count_std=('request_bit_string_count', 'std'),

        response_size=('response_size', 'mean'),
        response_size_std=('response_size', 'std'),

        response_node_count=('response_node_count', 'mean'),
        response_node_count_std=('response_node_count', 'std'),

        certificate_hash_count=('certificate_hash_count', 'mean'),
        certificate_hash_count_std=('certificate_hash_count', 'std'),

        consistency_proof_size=('consistency_proof_size', 'mean'),
        consistency_proof_size_std=('consistency_proof_size', 'std'),

        time_building_query=('time_building_query', 'mean'),
        time_building_query_std=('time_building_query', 'std'),

        time_send_receive=('time_send_receive', 'mean'),
        time_send_receive_std=('time_send_receive', 'std'),

        time_verification=('time_verification', 'mean'),
        time_verification_std=('time_verification', 'std'),

        time_consistency=('time_consistency', 'mean'),
        time_consistency_std=('time_consistency', 'std'),

        time_total=('time_total', 'mean'),
        time_total_std=('time_total', 'std'),
    ).agg(
        # collapse matrix
        "sum",
        axis="columns"
    )

    df_excluding_certificates = df[df['include_certificates'] == False]
    df_including_certificates = df[df['include_certificates'] == True]

    # request_size
    ax = cdf_plot(
        "request and response sizes in B",
        df['request_size'],
        None,
        f"{output_path}/request-size-cdf.png",
        base=2,
        labels=["request size", ""],
        plot=False
    )

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
        ax=ax
    )

    # request_bit_string_count
    cdf_plot(
        "request bit string count",
        df_excluding_certificates['request_bit_string_count'],
        df_including_certificates['request_bit_string_count'],
        f"{output_path}/request-bit-string-count-cdf.png",
        base=None
    )

    # response_node_count
    cdf_plot(
        "response node count",
        df_excluding_certificates['response_node_count'],
        df_including_certificates['response_node_count'],
        f"{output_path}/response-node-count-cdf.png",
        base=2
    )

    # response hash count
    cdf_plot(
        "response certificate hash count",
        df_excluding_certificates['certificate_hash_count'],
        df_including_certificates['certificate_hash_count'],
        f"{output_path}/response-hash-count-cdf.png",
        show_quantile=False,
        base=10
    )

    # consistency_proof_size
    # cdf_plot(
    #     "response consistency proof size",
    #     df_excluding_certificates['consistency_proof_size'],
    #     df_including_certificates['consistency_proof_size'],
    #     f"{output_path}/consistency-proof-size-cdf.png",
    # )

    # time_building_query
    ax = cdf_plot(
        "query time in s",
        df['time_building_query'],
        None,
        f"{output_path}/time-build-query-cdf.png",
        base=10,
        plot=False,
        labels=["query build time", ""]
    )

    # time_send_receive
    # cdf_plot(
    #     "request time in s",
    #     df_excluding_certificates['time_send_receive'],
    #     df_including_certificates['time_send_receive'],
    #     f"{output_path}/time-send-receive-cdf.png",
    # )

    # time_verification
    cdf_plot(
        "time to verify response in s",
        df_excluding_certificates['time_verification'],
        df_including_certificates['time_verification'],
        f"{output_path}/time-verification-cdf.png",
        ax=ax,
        plot=False,
        labels=[
            "verification time exc. certs",
            "verification time inc. certs"
        ],
        linestyles=["dashed", "dotted"]
    )

    # time_consistency
    # cdf_plot(
    #     "time to verify consistency in s",
    #     df_excluding_certificates['time_consistency'],
    #     df_including_certificates['time_consistency'],
    #     f"{output_path}/time-consistency-cdf.png",
    # )

    # time_total
    cdf_plot(
        "total request time in s",
        df_excluding_certificates['time_total'],
        df_including_certificates['time_total'],
        f"{output_path}/time-total-cdf.png",
        ax=ax,
        labels=[
            "total time exc. certs",
            "total time inc. certs"
        ],
        linestyles=["dashdot", (0, (3, 5, 1, 5, 1, 5))]
    )


if __name__ == '__main__':
    main()
