import click
import os
import shutil
import base64
from http.cookiejar import CookieJar
from typing import List
from urllib import request, parse
from urllib.error import HTTPError
from getpass import getpass
from tqdm import tqdm
import zipfile


def authenticate(e: HTTPError, credentials: str, cookie_jar: CookieJar):

    # install authorized opener
    authorized_opener = request.build_opener(
        request.HTTPCookieProcessor(cookie_jar)
    )
    authorized_opener.addheaders = [
        ('Authorization', f"Basic {credentials}")
    ]
    request.install_opener(authorized_opener)

    # authenticate
    request.urlopen(e.url)

    # reset opener
    cookie_opener = request.build_opener(
        request.HTTPCookieProcessor(cookie_jar)
    )
    request.install_opener(cookie_opener)


@click.command()
@click.argument('output_path', type=click.Path(exists=False))
@click.argument('username', type=str)
def main(
    output_path: str,
    username: str
):

    if not os.path.isdir(output_path):
        print(f"'{output_path}' is not a directory")

    with request.urlopen('https://www.opentopodata.org/datasets/aster30m_urls.txt') as f:
        urls: List[str] = f.read().decode('utf-8').split("\n")

    assert len(urls) == 22912

    password = getpass(
        f"Password for {username}@usgs.gov (NASA Earthdata Login): "
    )
    credentials = base64.b64encode(
        f"{username}:{password}".encode("ascii")).decode("ascii")

    files = os.listdir(output_path)

    for f in tqdm(
        files,
        total=len(files),
        desc="clean output dir"
    ):
        p = os.path.join(output_path, f)
        if os.path.isdir(p):
            shutil.rmtree(p)
        elif not f.endswith(".tif"):
            # delete all non tif files
            os.unlink(p)

    # install cookie handler
    cookie_jar = CookieJar()
    cookie_opener = request.build_opener(
        request.HTTPCookieProcessor(cookie_jar))
    request.install_opener(cookie_opener)

    for url in tqdm(
        urls,
        total=len(urls),
        desc="downloading aster30m"
    ):

        file_name: str = os.path.basename(parse.urlparse(url).path)
        zip_file_path = os.path.join(output_path, file_name)

        tif_file_name = f"{file_name.removesuffix('.zip')}_dem.tif"
        tif_file_path = os.path.join(output_path, tif_file_name)

        extract_dir = os.path.join(output_path, file_name.removesuffix('.zip'))

        if os.path.exists(tif_file_path):
            # already downloaded
            continue

        try:
            request.urlretrieve(url, zip_file_path)
        except HTTPError as e:
            if e.code == 401:
                authenticate(e, credentials, cookie_jar)
                request.install_opener(cookie_opener)
                # then retry
                request.urlretrieve(url, zip_file_path)
            else:
                raise

        if os.path.isfile(extract_dir):
            raise Exception(f"'{extract_dir}' is a file, delete or move it")

        if not os.path.isdir(extract_dir):
            os.mkdir(extract_dir)

        with zipfile.ZipFile(zip_file_path, 'r') as zip_ref:
            zip_ref.extractall(extract_dir)

        os.rename(os.path.join(extract_dir, tif_file_name), tif_file_path)

        os.unlink(zip_file_path)
        shutil.rmtree(extract_dir)


if __name__ == '__main__':
    main()
