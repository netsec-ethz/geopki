import pandas as pd


def load_sampling_map(
        sampling_map_path: str,
) -> pd.DataFrame:
    df = pd.read_csv(sampling_map_path)

    return df


def sample_df(
        df: pd.DataFrame,
        num_samples: int,
) -> pd.DataFrame:
    sample = df.sample(num_samples, replace=True)
    return sample.values


def sample(
        sampling_map_path: str,
        num_samples: int,
) -> list[tuple[float, float]]:
    df = load_sampling_map(sampling_map_path)

    return sample_df(df, num_samples)


def load_bit_string_sampling_map(
        sampling_map_path: str,
) -> pd.DataFrame:
    df = pd.read_parquet(sampling_map_path)

    return df
