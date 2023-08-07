import click
import pandas as pd
import numpy as np
from tqdm import tqdm
from typing import Dict, List
from shapely import Polygon, MultiPolygon
from collections import deque
import matplotlib.pyplot as plt
import geopandas as gpd
import os
import math
import sys

sys.path.insert(1, os.path.join(sys.path[0], '../../..'))  # noqa - prevent auto formatting
from coordinatez import DiscretizedVoxel, GeodeticCoordinate


def grow_initial_area(initial_area: DiscretizedVoxel, area: float, f_grow: float, plot=False):
    # while the area of the object is larger than a fraction of the grid's area
    # decrease the grid's size
    while (
        initial_area.x_precision > 1 and
        DiscretizedVoxel.from_bit_string_tuple(
            initial_area.to_bit_string_tuple()[0][:-1], ""
        ).to_shapely_area().area < area * f_grow
    ):
        if plot:
            x, y = initial_area.to_shapely_area().exterior.xy
            plt.plot(
                x,
                y,
                color="tab:orange",
                linestyle="solid",
                zorder=10,
                linewidth=1
            )

        # grow area by removing one bit
        initial_area = DiscretizedVoxel.from_bit_string_tuple(
            initial_area.to_bit_string_tuple()[0][:-1], ""
        )

    if plot:
        x, y = initial_area.to_shapely_area().exterior.xy
        plt.plot(
            x,
            y,
            color="tab:orange",
            linestyle="solid",
            zorder=10,
            linewidth=1
        )

    return initial_area


@click.command()
# the domain-location file
@click.argument('input_path', type=click.Path(exists=True))
@click.argument('output_path', type=click.Path(exists=False))
@click.option(
    '--f',
    '-f',
    'f_grow',
    type=float,
    default=1
)
@click.option('--voxels', 'voxels', flag_value=True, default=False)
def main(
    input_path: str,
    output_path: str,
    f_grow: float,
    voxels: bool,
):

    if input_path.endswith(".parquet"):
        df = pd.read_parquet(input_path)
    elif input_path.endswith(".csv"):
        df = pd.read_csv(input_path)
    else:
        raise Exception(f"Unkown input file type")

    if not output_path.endswith(".png"):
        raise Exception(f"output path has to end with .png")

    fig, ax = plt.subplots(figsize=(6, 6), dpi=300)
    fig.subplots_adjust(bottom=0.16, left=0.09, right=0.99)
    plt.axis('off')

    for row_i, (idx, row) in tqdm(
        enumerate(df.iterrows()),
        total=len(df),
        desc="find area"
    ):

        if row['domain'] is None or not isinstance(row['domain'], str):
            continue

        # china
        if row['certificate_id'] != "rel:270056":
            continue
        else:
            # only show mainland china
            row['list_of_multipolygons'] = [[
                row['list_of_multipolygons'][0][0]
            ]]

        # switzerland
        # if row['certificate_id'] != "rel:51701":
        #     continue

        # ethz
        # if not "way:192151232" in row['certificate_id']:
        #     continue

        # cab
        # if not "way:21976547" in row['certificate_id']:
        #     continue

        # relation:10002398
        # if not "rel:10002398" in row['certificate_id']:
        #     continue

        # relation:10006928
        # if not "rel:10006928" in row['certificate_id']:
        #     continue

        # large polygon
        # if len(row["list_of_multipolygons"][0]) != 1 or len(row["list_of_multipolygons"][0][0]) < 30 or len(row["list_of_multipolygons"][0][0]) > 50:
        #     continue

        print(row['certificate_id'])
        print(len(row["list_of_multipolygons"][0][0]))

        # print("found it")
        # print(row['list_of_multipolygons'])

        # 'certificate_id', 'list_of_multipolygons', 'parents', 'children', 'domain'
        valid_polygons = [
            polygon
            for polygon in row['list_of_multipolygons'][0]
            # exclude not proper areas
            if polygon[0] == polygon[-1]
        ]

        if len(valid_polygons) <= 0:
            continue

        # for each polygon perform the search separately
        initial_areas = [
            (
                DiscretizedVoxel.from_coordinate(
                    GeodeticCoordinate(
                        longitude=polygon[0]['lon'],
                        latitude=polygon[0]['lat'],
                        altitude=0
                    )
                ),
                Polygon(
                    [(x['lon'], x['lat']) for x in polygon]
                )
            )
            for polygon in valid_polygons
        ]

        intersecting_areas: List[str] = []
        # building: List[Polygon] = []

        # iterate over initial areas with they associated polygon
        # for each compute the intersecting voxels
        for initial_area, polygon in initial_areas:

            # print(initial_area.to_bit_string())
            # building.append(polygon)
            x, y = polygon.exterior.xy
            # plt.plot(x, y, color="black", linestyle=(0, (1, 1)), zorder=0)
            plt.plot(x, y, color="black", linestyle="solid", zorder=0)

            # walk around that voxel and find other intersecting ones
            # BFS
            visited: Dict[str, bool] = {}
            q = deque()
            q.append(
                grow_initial_area(
                    initial_area=initial_area,
                    # area=multi_polygon.area,
                    area=polygon.area,
                    f_grow=f_grow,
                    plot=(not voxels)
                )
            )

            while len(q) > 0:
                a: DiscretizedVoxel = q.popleft()
                bit_string = a.to_bit_string_tuple()[0]

                if bit_string in visited:
                    continue

                # mark as visited
                visited[bit_string] = True

                asa = a.to_shapely_area()

                # check for intersection
                if not (
                    asa.intersects(polygon)
                ):
                    # if plot:
                    #     x, y = asa.exterior.xy
                    #     plt.plot(x, y, color="tab:red")

                    continue

                # checkerboard pattern
                # width is 2
                is_even_column = (
                    a.x_min / (1 << (DiscretizedVoxel.X_BITS - a.x_precision))
                ) % 2 == 0
                is_even_row = (
                    a.y_min / (1 << (DiscretizedVoxel.Y_BITS - a.y_precision))
                ) % 2 == 0

                if ((is_even_row and is_even_column) or (not is_even_row and not is_even_column)) and not voxels:
                    # gpd.GeoSeries(asa).plot(
                    #     ax=ax,
                    #     facecolor="tab:olive",
                    #     alpha=0.5
                    # )
                    x, y = asa.exterior.xy
                    plt.plot(
                        x,
                        y,
                        color="tab:blue",
                        linestyle="dotted",
                        linewidth=1,
                        zorder=5
                    )
                else:
                    pass
                    # gpd.GeoSeries(asa).plot(
                    #     ax=ax,
                    #     facecolor="tab:blue",
                    #     alpha=0.5
                    # )

                # add to intersection list
                intersecting_areas.append(bit_string)

                # visit neighbors of a
                for dx in [-1, 0, 1]:
                    for dy in [-1, 0, 1]:

                        y_next = (
                            a.y_min + dy *
                            (1 << (DiscretizedVoxel.Y_BITS - a.y_precision))
                        )

                        x_next = (
                            a.x_min + dx *
                            (1 << (DiscretizedVoxel.X_BITS - a.x_precision))
                        ) % DiscretizedVoxel.C_X

                        if y_next < 0:
                            # the y-coordinate 'flips', we can account for this
                            # by only rotating around x and set y to 0
                            y_next = 0
                            # if we overflow, the x coordinate wraps around
                            x_next = (
                                x_next + math.floor(DiscretizedVoxel.C_X / 2)
                            ) % DiscretizedVoxel.C_X
                        elif y_next >= DiscretizedVoxel.C_Y:
                            # the y-coordinate 'flips', we can account for this
                            # by rotating around x and set y to C_Y - step size = original y
                            y_next = a.y_min
                            # if we overflow the x coordinate wraps around
                            x_next = (
                                x_next + math.floor(DiscretizedVoxel.C_X / 2)
                            ) % DiscretizedVoxel.C_X

                        # clear bottom bits of the x coordinate, might be messed up after wrapping around
                        bl = len(bin(x_next)[2:]) - a.x_precision
                        if bl > 0:
                            x_next = (x_next >> bl) << bl

                        q.append(
                            DiscretizedVoxel(
                                x_min=x_next,
                                x_precision=a.x_precision,
                                y_min=y_next,
                                y_precision=a.y_precision,
                                # z will stay the same
                                z_min=a.z_min,
                                z_precision=a.z_precision,
                            )
                        )

        # after computing the intersecting voxels, prune them and add the certificates to a map
        bit_string_idx = 0
        border = None

        count = 0

        while bit_string_idx < len(intersecting_areas):
            bit_string = intersecting_areas[bit_string_idx]
            # increase idx for the next iteration
            bit_string_idx += 1

            skip = False
            # iterate over all prefixes of that bitstring from largest/shortest to smallest/longest
            for i in range(1, len(bit_string)):
                # check if any of its prefixes (larger areas) is also part of intersecting_areas
                if bit_string[:i] in intersecting_areas:
                    # if it is, ignore this one as the certificate will be included in the larger/shorter
                    # prefix
                    skip = True
                    break

            if skip:
                # ignore by skipping over this index
                continue

            # check if area can be merged with neighbor
            bit_string_neighbor = (
                bit_string[:-1] + ("0" if bit_string[-1:] == "1" else "1")
            )
            if bit_string_neighbor in intersecting_areas:
                # yes it can. ignore current bit_string by skipping (continue)
                # but remove neighbor to prevent duplicate area
                intersecting_areas.remove(bit_string_neighbor)
                # and add parent at the end of the list to make sure duplicate test is performed with parent again
                intersecting_areas.append(bit_string[:-1])

                continue

            # from this point on bit_string is sucessfully taken
            count += 1

            poly = DiscretizedVoxel.from_bit_string_tuple(
                bit_string, ""
            ).to_shapely_area()

            if voxels:
                x, y = poly.exterior.xy
                plt.plot(
                    x,
                    y,
                    color="tab:blue",
                    linestyle="solid",
                    linewidth=2,
                    zorder=5
                )
            else:
                if border == None:
                    border = poly
                else:
                    border = border.union(poly)

            #  plt.plot(x, y, color="tab:purple", linestyle="solid", linewidth=1)

        # plot building
        # for polygon in building:
        #     x, y = polygon.exterior.xy
        #     plt.plot(x, y, color="black", linestyle=(0, (1, 1)))

        # plot boundary
        if not voxels:
            if isinstance(border, Polygon):
                x, y = border.exterior.xy
                plt.plot(
                    x,
                    y,
                    color="tab:blue",
                    linestyle="solid",
                    linewidth=2,
                    zorder=5
                )
            elif isinstance(border, MultiPolygon):
                for polygon in border.geoms:
                    x, y = polygon.exterior.xy
                    plt.plot(
                        x,
                        y,
                        color="tab:blue",
                        linestyle="solid",
                        linewidth=2,
                        zorder=5
                    )

        print("xlim:", plt.xlim())
        print("ylim:", plt.ylim())

        bottom, top = plt.ylim()

        # https://geopandas.org/en/stable/docs/reference/api/geopandas.GeoSeries.plot.html#geopandas.GeoSeries.plot
        f = 1.0 / np.cos((bottom + (top - bottom) / 2) * np.pi / 180)
        plt.gca().set_aspect(f)

        # plt.xlim(8.547981262207031, 8.549491882324219)
        # plt.ylim(47.3778018951416, 47.37893486022949)
        plt.tight_layout()
        plt.savefig(output_path, bbox_inches='tight')
        plt.close()

        print(f"voxels: {count}")
        exit()

        # if row_i > 1:
        #     assert "1" in bit_string_map
        #     break
        # print(len(intersecting_areas))
        # exit()


if __name__ == '__main__':
    main()
