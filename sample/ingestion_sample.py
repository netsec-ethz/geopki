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
from cryptography import x509
from cryptography.hazmat.primitives.serialization import Encoding

#
# How to:
#
# ```bash
# source ./venv/bin/activate
# python3 sample/ingestion_sample.py
# ```
#
# Release is done by the script, too.
#
# Adjust the addded geo cert by changing the `ASSEMBLE GEO CERT` section.
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
        id: str,
        # what kind of geo cert this is (e.g. a `wifi` geo cert)
        # validity of geo cert
        not_valid_after: datetime,
        # as lat/lon coordinates
        list_of_multipolygons: List[MultiPolygon],
        # in meters
        list_of_altitudes: List[Tuple[float, float]],
        domain: str,
        payload: Any,
    ) -> None:
        self.id = id
        self.not_valid_after = not_valid_after
        self.list_of_multipolygons = list_of_multipolygons
        self.list_of_altitudes = list_of_altitudes
        self.domain = domain
        self.payload = payload

    def to_cert(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "not_valid_after": self.not_valid_after.isoformat(
                sep=" ", timespec="seconds"
            ),
            "areas": [
                json.loads(to_geojson(multipolygon))
                for multipolygon in self.list_of_multipolygons
            ],
            "areas_altitude": list(self.list_of_altitudes),
            "domain": self.domain,
            "payload": self.payload,
        }

    def hash(self) -> bytes:
        return hashlib.sha256(self.to_cert()).digest()


# def x509_cert_from_file(file_name: str) -> x509.Certificate:
#     if not file_name.endswith(".pem"):
#         raise ValueError("X509 Certificate must be in .pem format.")
#     with open(file_name, "rb") as cert_file:
#         x509_cert_bytes = cert_file.read()
#     return x509.load_pem_x509_certificate(x509_cert_bytes)


def pem_to_cert_path(pem: str) -> list[str]:
    cert_path = []
    
    pem_blocks = pem.split('-----END CERTIFICATE-----')
    for pem_block in pem_blocks:
        if pem_block.strip():
            pem_block += '-----END CERTIFICATE-----'
            cert = x509.load_pem_x509_certificate(pem_block.encode('utf-8'))
            cert_path.append(cert)

    return cert_path


def main():

    # BEGIN: ASSEMBLE GEO CERT

    date = datetime.datetime.today().strftime("%Y-%m-%d")
    uid = hashlib.sha256(str(time.time()).encode("ascii")).digest().hex()[:8]

    # ETH
    lat = 47.376389  # °N
    lon = 8.548056  # °E
    radius = 1  # ° of a square

    domain = "wifi"
    # payload for Wi-Fi geo cert: SSID and link (~WPA) cert path (as PEM)
    with open(f"{CURRENT_DIR}/certificates/eduroam-ethz-extracted.pem", 'rb') as pem_file:
        pem_path_str = pem_file.read().decode('utf-8')

    cert_path = pem_to_cert_path(pem_path_str)
    payload = {
        "ssid": "eduroam",
        "wpaCertPathPem": pem_path_str,
    }

    geoCert = GeoCertificate(
        id=f"{lat}N-{lon}E_{uid}",
        not_valid_after=cert_path[0].not_valid_after,
        list_of_multipolygons=[
            MultiPolygon(
                [
                    # must follow the right-hand rule with respect to the area it bounds (i.e. exterior rings are counterclockwise, holes are clockwise)
                    Polygon(
                        [
                            (lon - radius, lat - radius),
                            (lon + radius, lat - radius),
                            (lon + radius, lat + radius),
                            (lon - radius, lat + radius),
                        ]
                    )
                ]
            )
        ],
        list_of_altitudes=[(MIN_ALTITUDE, MAX_ALTITUDE)],
        domain=domain,
        payload=payload,
    )

    # END: ASSEMBLE GEO CERT

    with tempfile.NamedTemporaryFile() as fp:
        # write query to temporary file
        fp.write(json.dumps([geoCert.to_cert()]).encode("utf-8"))
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
            f"--insertion-key={INSERTION_KEY}",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=CURRENT_DIR,
    )
    print(p.stdout.read().decode("ascii"))
    print(p.stderr.read().decode("ascii"), file=sys.stderr)


if __name__ == "__main__":
    main()
