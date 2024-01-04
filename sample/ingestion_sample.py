import sys
from typing import Dict, Any, Union, Optional, List, Tuple
from shapely import MultiPolygon, Polygon, Point, to_geojson
from shapely.geometry.polygon import orient
import hashlib
import subprocess
import os
import tempfile
import json
import time
import datetime

#
# How to:
#
# ```bash
# source ./venv/bin/activate
# python3 performance/ingestion/ingestion_sample.py
# ```
#
# Release is done by the script, too.
#

SERVER_ADDRESS = "http://127.0.0.1:1234"
INSERTION_KEY = "abc"

MIN_ALTITUDE = -11000
MAX_ALTITUDE = 21767

FILE_PATH = os.path.realpath(__file__)
CURRENT_DIR = os.path.dirname(FILE_PATH)
GEOPKI_DIR = os.path.dirname(CURRENT_DIR)


class GeoCertificate:
    def __init__(
        self,
        certificate_id: str,
        # as lat/lon coordinates
        list_of_multipolygons: List[MultiPolygon],
        # in meters
        list_of_altitudes: List[Tuple[float, float]],
        x509_certificate: str,
        not_valid_after: datetime,
    ) -> None:
        self.certificate_id = certificate_id
        self.list_of_multipolygons = list_of_multipolygons
        self.list_of_altitudes = list_of_altitudes
        self.x509_certificate = x509_certificate
        self.not_valid_after = not_valid_after

    def to_cert(self) -> dict[str, Any]:
        return {
            "x509_certificate": self.x509_certificate,
            "areas": [
                json.loads(to_geojson(multipolygon))
                for multipolygon in self.list_of_multipolygons
            ],
            "areas_altitude": list(self.list_of_altitudes),
            "certificate_id": self.certificate_id,
            "not_valid_after": self.not_valid_after.isoformat(
                sep=" ", timespec="seconds"
            )
            # "not_valid_after": "2030-01-01 00:00:00+00",
        }

    def hash(self) -> bytes:
        return hashlib.sha256(self.to_cert()).digest()


def main():
    date = datetime.datetime.today().strftime("%Y-%m-%d")
    uid = hashlib.sha256(str(time.time()).encode("ascii")).digest().hex()[:8]

    # ETH
    lon = 47.376389  # N
    lat = 8.548056  # E
    radius = 10  # m

    # certificate as .pem file
    x509_file = f"{CURRENT_DIR}/certificates/eduroam-ethz-extracted.pem"

    if not x509_file.endswith(".pem"):
        raise ValueError("X509 Certificate must be in .pem format.")
    with open(x509_file, "rb") as cert_file:
        x509_cert_bytes = cert_file.read()

    from cryptography import x509
    x509_cert = x509.load_pem_x509_certificate(x509_cert_bytes)

    cert = GeoCertificate(
        certificate_id=f"ingestion:{date}-{uid}",
        list_of_multipolygons=[
            MultiPolygon(
                [
                    Polygon(
                        [
                            (lat - radius, lon - radius),
                            (lat - radius, lon + radius),
                            (lat + radius, lon + radius),
                            (lat + radius, lon - radius),
                        ]
                    )
                ]
            )
        ],
        list_of_altitudes=[(MIN_ALTITUDE, MAX_ALTITUDE)],
        x509_certificate=x509_cert_bytes.decode("utf-8"),
        not_valid_after=x509_cert.not_valid_after
    )

    with tempfile.NamedTemporaryFile() as fp:
        # write query to temporary file
        fp.write(json.dumps([cert.to_cert()]).encode("utf-8"))
        fp.flush()

        print("INSERT CERTIFICATE")
        p = subprocess.Popen(
            [
                f"{GEOPKI_DIR}/dist/geopki-client-ingestion",
                f"--address={SERVER_ADDRESS}",
                f"--insertion-key={INSERTION_KEY}",
                f"--certificates={fp.name}",
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            cwd=CURRENT_DIR,
        )

        print(p.stdout.read().decode("ascii"))
        print(p.stderr.read().decode("ascii"), file=sys.stderr)
    
    # release
    print("RELEASE")
    p = subprocess.Popen(
        [
            "go",
            "run",
            f"{GEOPKI_DIR}/cmd/release",
            f"--address={SERVER_ADDRESS}",
            f"--insertion-key={INSERTION_KEY}"
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=CURRENT_DIR,
    )
    print(p.stdout.read().decode("ascii"))
    print(p.stderr.read().decode("ascii"), file=sys.stderr)


if __name__ == "__main__":
    main()
