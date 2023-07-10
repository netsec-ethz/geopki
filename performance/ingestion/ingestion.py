from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import MultiPolygon, Polygon, Point, to_geojson
from shapely.geometry.polygon import orient
import click
import hashlib
import pandas as pd
import numpy as np
import subprocess
import os
import tempfile
import json
import time
from urllib.request import Request, urlopen
from datetime import datetime
from geopy import distance

from tqdm import tqdm

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)

CERTIFICATE_BATCH_SIZES = [10, 100, 1000, 10000]
SQUARE_SIDE_LENGTH_M = 30

MIN_ALTITUDE = -10000
MAX_ALTITUDE = 22767

MIN_HEIGHT = 3
MAX_HEIGHT = 30


class GeoCertificate:
    def __init__(
        self,
        certificate_id: str,
        list_of_multipolygons: List[MultiPolygon],
        list_of_altitudes: List[Tuple[float, float]],
    ) -> None:
        self.certificate_id = certificate_id
        self.list_of_multipolygons = list_of_multipolygons
        self.list_of_altitudes = list_of_altitudes

    def to_cert(self) -> dict[str, Any]:
        return {
            'domain': 'netsec.ethz.ch',
            'areas': [
                json.loads(
                    to_geojson(multipolygon)
                )
                for multipolygon in self.list_of_multipolygons
            ],
            'areas_altitude': list(self.list_of_altitudes),
            'certificate_id': self.certificate_id,
            'not_valid_after': "2030-01-01 00:00:00+00",
        }

    def hash(self) -> bytes:
        return hashlib.sha256(self.to_cert()).digest()


def sample_polygon_in_polygon(polygon: Polygon) -> Polygon:
    minX, minY, maxX, maxY = polygon.bounds

    while True:
        # rejection sampling
        bottom_left = Point(np.random.uniform(minX, maxX),
                            np.random.uniform(minY, maxY))

        if polygon.contains(bottom_left):

            top_left = distance.distance(
                meters=SQUARE_SIDE_LENGTH_M
            ).destination(
                # (latitude, longitude)
                (bottom_left.y, bottom_left.x),
                bearing=0
            )

            top_right = distance.distance(
                meters=SQUARE_SIDE_LENGTH_M
            ).destination(
                # (latitude, longitude)
                (top_left.latitude, top_left.longitude),
                bearing=90
            )

            bottom_right = distance.distance(
                meters=SQUARE_SIDE_LENGTH_M
            ).destination(
                # (latitude, longitude)
                (bottom_left.y, bottom_left.x),
                bearing=90
            )

            p = Polygon([
                (bottom_left.x, bottom_left.y),
                (bottom_right.longitude, bottom_right.latitude),
                (top_right.longitude, top_right.latitude),
                (top_left.longitude, top_left.latitude),
                (bottom_left.x, bottom_left.y),
            ])

            p = orient(p)

            return p


def sample_random_altitude() -> tuple[float, float]:
    height = np.random.uniform(MIN_HEIGHT, MAX_HEIGHT)
    altitude = np.random.uniform(MIN_ALTITUDE, MAX_ALTITUDE - height)

    return (altitude, altitude + height)


@click.command()
@click.argument('website_density_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
@click.option(
    '--address',
    '-a',
    'address',
    type=str,
    default='http://localhost:1234',
)
@click.option(
    '--insertion-key',
    '-i',
    'insertion_key',
    type=str,
    default='http://localhost:1234',
)
@click.option(
    '--repetitions',
    '-r',
    'repetitions',
    type=int,
    default=10
)
@click.option('--with-altitude', 'with_altitude', flag_value=True, default=False)
def main(
    website_density_path: str,
    output_path: str,
    address: str,
    insertion_key: str,
    repetitions: int,
    with_altitude: bool,
):

    if not os.path.isfile(website_density_path):
        raise Exception(f"Website density path does to point to a file")

    if not website_density_path.endswith(".parquet"):
        raise Exception(f"Website density path does to end in '.parquet'")

    df = pd.read_parquet(website_density_path)

    df['polygon'] = df['polygon'].apply(
        lambda points:
        Polygon([
            (p[1], p[0])
            for p in points
        ])
    )

    if os.path.isdir(output_path):
        raise Exception(f"Output path points to a directory")

    if not output_path.endswith(".csv"):
        raise Exception(f"Output path extension has to be .csv")

    if os.path.isfile(output_path):
        f = open(output_path, "a")
    else:
        f = open(output_path, "w")
        f.write(
            f"certificate_count,time,success\n"
        )
        f.flush()

    # probability 0 if osm_website_element_count == 0
    df['weight'] = df['osm_website_element_count']

    # very small probability if osm_website_element_count == 0
    # df['weights'] = df['osm_website_element_count'] + 1
    date = datetime.today().strftime('%Y-%m-%d')
    uid = hashlib.sha256(str(time.time()).encode("ascii")).digest().hex()[:8]

    total_iterations = len(CERTIFICATE_BATCH_SIZES) * repetitions

    for i, batch_size in tqdm(
        (
            (i, batch_size)
            for batch_size in CERTIFICATE_BATCH_SIZES
            for i in range(repetitions)
        ),
        total=total_iterations
    ):

        sample = df.sample(
            n=batch_size,
            weights='weight',
            random_state=1,
            replace=True
        )

        # for each sample, sample a point within the polygon
        sample['sample_polygon'] = sample['polygon'].apply(
            sample_polygon_in_polygon
        )
        certificate_polygons: list[Polygon] = sample['sample_polygon'].values

        certificates = [
            GeoCertificate(
                certificate_id=f"ingestion:{date}-{uid}-{batch_size}-{i}",
                list_of_multipolygons=[MultiPolygon([polygon])],
                list_of_altitudes=[
                    sample_random_altitude()
                    if with_altitude else
                    (MIN_ALTITUDE, MAX_ALTITUDE)
                ],
            )
            for polygon in certificate_polygons
        ]

        with tempfile.NamedTemporaryFile() as fp:

            # write queries to temporary file
            fp.write(
                json.dumps([
                    cert.to_cert()
                    for cert in certificates
                ]).encode("utf-8")
            )
            # flush to disk
            fp.flush()

            # print(f"go with {time}, {threads}, {fp.name}")
            p = subprocess.Popen(
                [
                    f"../../dist/geopki-client-ingestion",
                    f"--address={address}",
                    f"--insertion-key={insertion_key}",
                    f"--certificates={fp.name}",
                ],
                stdout=subprocess.PIPE,
                cwd=CURRENT_DIR
            )

            f.write(p.stdout.read().decode("ascii"))
            f.flush()

    f.close()


if __name__ == '__main__':
    main()
