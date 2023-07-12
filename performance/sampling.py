import pandas as pd


def sample(
        sampling_map_path: str,
        num_samples: int,
) -> list[tuple[float, float]]:
    df = pd.read_csv(sampling_map_path)
    df.sample(num_samples, replace=True)

    return df.values
