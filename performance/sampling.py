from shapely.geometry import Polygon, Point, GeometryCollection, shape
import numpy as np
import json
import random


def sample_point_in_polygon(polygon: Polygon) -> tuple[float, float]:
    "https://www.matecdev.com/posts/random-points-in-polygon.html"
    minX, minY, maxX, maxY = polygon.bounds

    while True:
        # rejection sampling
        sample = Point(np.random.uniform(minX, maxX),
                       np.random.uniform(minY, maxY))
        if polygon.contains(sample):
            return sample.x, sample.y


def sample(
        sampling_map_path: str,
        num_samples: int,
) -> list[tuple[float, float]]:
    f = open(sampling_map_path)
    sampling_map_features = json.load(f)["features"]
    f.close()

    sampling_map = GeometryCollection(
        [
            shape(feature["geometry"]).buffer(0)
            for feature in sampling_map_features
        ]
    )

    samples: list[tuple[float, float]] = []
    for _ in range(num_samples):
        shape = sampling_map.geoms[random.randint(
            0, len(sampling_map.geoms) - 1)]

        if not isinstance(shape, Polygon):
            raise Exception(f"unsupported shape type '{shape}'")

        samples.append(sample_point_in_polygon(shape))

    return samples
