import os

import pandas as pd
import seaborn as sns
import matplotlib.pyplot as plt
from matplotlib.ticker import MaxNLocator
from matplotlib.colors import LogNorm

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

fig, ax = plt.subplots(figsize=(6, 3), dpi=300)
fig.subplots_adjust(bottom=0.16, left=0.09, right=0.99)
ax.set_xscale("log", base=2)
ax.set_xlabel("number of parallel processes")
ax.set_yscale("linear")
ax.set_ylabel("queries per second")

file_to_fmt = {
    "performance-evaluation-spatial.csv": "o",
    "performance-evaluation-bitstring.csv": "x",
    "performance-evaluation-bitstring-int-f1.csv": "h",
}

file_to_label = {
    "performance-evaluation-spatial.csv": "spatial",
    "performance-evaluation-bitstring.csv": "bit strings",
    "performance-evaluation-bitstring-int-f1.csv": "bit string integers",
}

file_to_color = {
    "performance-evaluation-spatial.csv": "#3498db",
    "performance-evaluation-bitstring.csv": "#e74c3c",
    "performance-evaluation-bitstring-int-f1.csv": "#27ae60",
}

files = file_to_fmt.keys()
for file in files:
    if not file.endswith(".csv"):
        continue

    df = pd.read_csv(os.path.join(CURRENT_DIR, file))

    df['queries_per_second'] = df['executed_queries'] / df['time_s']

    df = df.groupby(
        by=['num_threads', 'query_radius_m']
    ).agg(
        queries_per_second=('queries_per_second', 'mean'),
        queries_per_second_std=('queries_per_second', 'std'),
    ).reset_index()

    print(file)
    print(df)
    print("-" * 80)

    for r in df['query_radius_m'].unique():
        x = df[(df['query_radius_m'] == r)]

        ax.errorbar(
            x['num_threads'],
            x['queries_per_second'],
            yerr=x['queries_per_second_std'],
            fmt=file_to_fmt[file],
            fillstyle='none',
            label=file_to_label[file],
            color=file_to_color[file]
        )

# plt.grid()
plt.legend(loc="upper left")
plt.savefig(f"{CURRENT_DIR}/plot.png")
plt.close()

fig, ax = plt.subplots(figsize=(6, 3), dpi=300)
fig.subplots_adjust(bottom=0.16, left=0.06, right=0.99)
ax.set_xlabel("SMT depth")
ax.set_yscale("linear")
# ax.set_ylabel("CDF")

df = pd.read_csv(os.path.join(CURRENT_DIR, "depth.csv"))
df['smt_depth'] = df['smt_depth_xy'] + df['smt_depth_z']

df.sort_values(
    by=['smt_depth'],
    ascending=False,
    inplace=True
)


def get_stats_df(column):
    # https://stackoverflow.com/a/54317197
    s = pd.Series(column.values, name='value')

    stats_df = pd.DataFrame(s).groupby('value')['value'] \
        .agg('count') \
        .pipe(pd.DataFrame) \
        .rename(columns={'value': 'frequency'})

    # PDF
    stats_df['pdf'] = stats_df['frequency'] / sum(stats_df['frequency'])

    # CDF
    stats_df['cdf'] = stats_df['pdf'].cumsum()
    stats_df = stats_df.reset_index()

    return stats_df


def plot_cdf(column, quantile=True):
    stats_df = get_stats_df(column)

    p = plt.plot(
        stats_df['value'],
        stats_df['cdf'],
    )
    color = p[0].get_color()

    if quantile:

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
            ninety_five * 0.98,
            0.85,
            # f'({ninety_five}, {ninety_five_p.round(2)})',
            f'{int(ninety_five) if ninety_five.is_integer() else ninety_five.round(4)}',
            rotation=0,
            color=color
        )


def plot_pdf(column):
    stats_df = get_stats_df(column)

    p = plt.scatter(
        stats_df['value'],
        stats_df['pdf'],
        s=1,
        marker="x",
    )


plot_cdf(df['smt_depth'])
plt.savefig(f"{CURRENT_DIR}/depth.png")
plt.close()

# cdf(df['smt_depth_xy'])
# cdf(df['smt_depth_z'])

df = pd.read_csv(os.path.join(CURRENT_DIR, "leaves.csv"))
df = df.groupby(
    by=['smt_depth_xy', 'smt_depth_z']
).agg(
    count=('smt_depth_xy', 'count')
).sort_values(
    by=['count'],
    ascending=False
)

for xy_depth in range(52):
    for z_depth in range(16):
        if not (xy_depth, z_depth) in df.index:
            df.loc[(xy_depth, z_depth), 'count'] = 0

df = df.reset_index()

fig, ax = plt.subplots(figsize=(6, 3), dpi=300)
fig.subplots_adjust(bottom=0.16, left=0.09, right=1.05)

ax.set_xlabel("surface tree depth")

ax.set_ylabel("altitude subtree depth")

pivoted_data = df.pivot_table(
    index='smt_depth_z',
    columns='smt_depth_xy',
    values='count',
    fill_value=0
)

# print(pivoted_data)
# exit()

ax = sns.heatmap(
    pivoted_data,
    ax=ax,
    norm=LogNorm(),
    cmap="gray",
)

ax.patch.set(hatch='xx', edgecolor='black')

# ax.set_ylim(bottom=0, top=15)
# ax.set_xlim(left=0, right=51)

xy_depth_ticks = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 51]
plt.xticks(xy_depth_ticks, xy_depth_ticks, rotation=0)

z_depth_ticks = [0, 3, 6, 9, 12, 15]
plt.yticks()
plt.yticks(z_depth_ticks, z_depth_ticks, rotation=0)

plt.xlabel("surface tree depth")
plt.ylabel("altitude subtree depth")

ax.invert_yaxis()

for _, spine in ax.spines.items():
    spine.set_visible(True)
    spine.set_linewidth(1)


plt.savefig(f"{CURRENT_DIR}/leaf-distribution.png")
plt.close()


df = pd.read_csv(os.path.join(CURRENT_DIR, "altitude.csv"))

df['altitude'] = df['altitude']
df['altitude'] = df['altitude'].round(0).astype(int)

fig, ax = plt.subplots(figsize=(6, 3), dpi=300)
fig.subplots_adjust(bottom=0.16, left=0.06, right=0.99)

plot_cdf(df['altitude'], quantile=False)

ylimits = ax.get_ylim()

ax.vlines(
    x=[264],
    ymin=ylimits[0],
    ymax=ylimits[1],
    color='tab:blue',
    linestyle='dotted',
    linewidth=1
)

plt.savefig(f"{CURRENT_DIR}/altitude-cdf.png")
plt.close()

fig, ax = plt.subplots(figsize=(6, 3), dpi=300)
fig.subplots_adjust(bottom=0.16, left=0.1, right=0.99)
# ax.set_xscale("log")

plot_pdf(df['altitude'])

ylimits = ax.get_ylim()

ax.vlines(
    x=[264],
    ymin=ylimits[0],
    ymax=ylimits[1],
    color='tab:blue',
    linestyle='dotted',
    linewidth=1
)

plt.savefig(f"{CURRENT_DIR}/altitude-pdf.png")
plt.close()
