import os

import pandas as pd
import matplotlib.pyplot as plt

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

fig, ax = plt.subplots(dpi=300)
ax.set_xscale("log", base=2)
ax.set_xlabel("number of parallel processes")
ax.set_yscale("linear")
ax.set_ylabel("queries per second")

file_to_fmt = {
    "performance-evaluation-spatial.csv": "o",
    "performance-evaluation-bitstring.csv": "x",
    "performance-evaluation-bitstring-exclude-comp.csv": "+",
    "performance-evaluation-bitstring-int.csv": "h",
    "performance-evaluation-bitstring-int-exclude-comp.csv": "d",
}

file_to_label = {
    "performance-evaluation-spatial.csv": "spatial",
    "performance-evaluation-bitstring.csv": "bit strings",
    "performance-evaluation-bitstring-exclude-comp.csv": "bit strings *",
    "performance-evaluation-bitstring-int.csv": "bit string integers",
    "performance-evaluation-bitstring-int-exclude-comp.csv": "bit string integers *",
}

file_to_color = {
    "performance-evaluation-spatial.csv": "#3498db",
    "performance-evaluation-bitstring.csv": "#f39c12",
    "performance-evaluation-bitstring-exclude-comp.csv": "#e74c3c",
    "performance-evaluation-bitstring-int.csv": "#2ecc71",
    "performance-evaluation-bitstring-int-exclude-comp.csv": "#27ae60",
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
