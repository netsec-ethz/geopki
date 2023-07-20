from typing import List, Dict, Tuple
import math
from collections import deque
from shapely import Point, Polygon, MultiPolygon, from_wkt
import matplotlib.pyplot as plt
from geopy import distance
import os
import sys

sys.path.insert(1, os.path.join(sys.path[0], '..'))  # noqa - prevent auto formatting
from coordinates import ZOrderBitString, GeodeticCoordinate


def sphere_to_2d_binary_strings(
        center: GeodeticCoordinate,
        radius_m: float
) -> List[str]:
    # add maximum diagonal of a voxel to the radius
    # this over-approximates the sphere but ensures we only have to
    # check the voxel's vertices to be within the sphere's radius
    MAX_VOXEL_DIAGONAL = math.ceil(math.sqrt(3))

    radius_m += MAX_VOXEL_DIAGONAL

    cartesian_center = center.to_cartesian()

    q = deque()
    center_voxel = ZOrderBitString.from_coordinate(
        coordinate=center
    )
    q.append(center_voxel)

    memo: Dict[str, bool] = {}
    memo[center_voxel.to_bit_string()] = True

    binary_strings: List[str] = []

    while len(q) > 0:
        s: ZOrderBitString = q.popleft()

        bit_string = s.to_bit_string()[:-15]
        neighbor = bit_string[:-1] + ("0" if bit_string[-1:] == "1" else "1")

        while neighbor in binary_strings:
            # merge with neighbor
            binary_strings.remove(neighbor)
            bit_string = bit_string[:-1]

            # check higher level neighbor
            neighbor = bit_string[:-1] + \
                ("0" if bit_string[-1:] == "1" else "1")

        binary_strings.append(bit_string)

        # print(p.eucledian_distance_voxel_point(cartesian_center))
        # print(p.to_coordinate())
        # if it is within the radius, append to list and visit all neighbors

        for dx in [-1, 0, 1]:
            for dy in [-1, 0, 1]:
                for dz in [0]:  # [-1, 0, 1]:

                    x = s.x_min + dx
                    y = s.y_min + dy
                    z = s.z_min + dz

                    # if the poles are crossed, the longitude flips by 180 degrees
                    if y < 0:
                        y = -y
                        # it is fine if we overflow, the x coordinate wraps around
                        x += math.floor(ZOrderBitString.C_X / 2)
                    elif y >= ZOrderBitString.C_Y:
                        y = ZOrderBitString.C_Y - \
                            (y - ZOrderBitString.C_Y)
                        # it is fine if we overflow, the x coordinate wraps around
                        x += math.floor(ZOrderBitString.C_X / 2)

                    # the longitudal axis wraps around
                    x = x % ZOrderBitString.C_X

                    # abort if out of bounds
                    if z < 0 or z > ZOrderBitString.C_Z:
                        continue

                    # check if the voxel is within the sphere's radius
                    p = ZOrderBitString(
                        x_min=x,
                        x_precision=ZOrderBitString.X_BITS,
                        y_min=y,
                        y_precision=ZOrderBitString.Y_BITS,
                        z_min=z,
                        z_precision=ZOrderBitString.Z_BITS
                    )

                    p_bit_string = p.to_bit_string()

                    if p_bit_string in memo:
                        # this voxel was already visited
                        continue

                    # if not, mark as visited
                    memo[p_bit_string] = True

                    if p.eucledian_distance_voxel_point(cartesian_center) > radius_m:
                        # if not, skip this one
                        continue

                    # if it is, append to the queue
                    q.append(p)

    # distance.distance(
    #     kilometers=scan_unit_size_km
    # ).destination(
    #     (s, w),
    #     bearing=0
    # ).latitude
    return binary_strings


def grow_area(initial_area: ZOrderBitString, area: float, f: float, plot=False):
    # while the area of the object is larger than a fraction of the grid's area
    # decrease the grid's size
    while (
        initial_area.x_precision > 1 and
        ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        ).to_shapely_area().area < area * f
    ):
        if plot:
            x, y = initial_area.to_shapely_area().exterior.xy
            plt.plot(x, y, color="orange")

        # grow area by removing one bit
        initial_area = ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        )

    return initial_area


def sphere_to_coarse_2d_binary_strings(
        center: GeodeticCoordinate,
        radius_m: float,
        f_grow: float,
        f_min: float,
        plot: bool
) -> List[str]:
    # approximate circle, accuracy is less important as this is computed by the client
    # longitudes per meter is always greater than latitude per meter

    positions = (
        distance.distance(
            meters=radius_m
        ).destination(
            (center.latitude, center.longitude),
            bearing=bearing
        )

        for bearing in [45, 90, 135, 225, 270]
    )

    radius_lon_lat = math.sqrt(
        max(
            (p.longitude - center.longitude) ** 2 +
            (p.latitude - center.latitude) ** 2
            for p in positions
        )
    )
    circle = Point(center.longitude, center.latitude).buffer(radius_lon_lat)

    if plot:
        x, y = circle.exterior.xy
        plt.plot(x, y, color="b")

    # this will be the list of bitstrings of the chosen size
    intersecting_areas: List[str] = []
    results: List[str] = []

    visited: Dict[str, bool] = {}
    q = deque()
    q.append(
        grow_area(
            initial_area=ZOrderBitString.from_coordinate(center),
            area=circle.area,
            f=f_grow,
            plot=plot
        )
    )

    while len(q) > 0:
        a: ZOrderBitString = q.popleft()
        bit_string = a.to_bit_string()

        if bit_string in visited:
            continue

        # mark as visited
        visited[bit_string] = True

        asa = a.to_shapely_area()

        # check for intersection. always take the first area
        if len(intersecting_areas) > 0 and not (
            # asa.intersects(circle)
            asa.intersection(circle).area > f_min * asa.area
        ):
            # if plot:
            #     x, y = asa.exterior.xy
            #     plt.plot(x, y, color="r")

            continue

        # if plot:
        #     x, y = asa.exterior.xy
        #     plt.plot(x, y, color="purple", linewidth=5)

        # add to intersection list
        intersecting_areas.append(bit_string)

        # visit neighbors of a
        for dx in [-1, 0, 1]:
            for dy in [-1, 0, 1]:

                y_next = (
                    a.y_min + dy *
                    (1 << (ZOrderBitString.Y_BITS - a.y_precision))
                )

                x_next = (
                    a.x_min + dx *
                    (1 << (ZOrderBitString.X_BITS - a.x_precision))
                ) % ZOrderBitString.C_X

                if y_next < 0:
                    # the y-coordinate 'flips', we can account for this
                    # by only rotating around x and set y to 0
                    y_next = 0
                    # if we overflow, the x coordinate wraps around
                    x_next = (
                        x_next + math.floor(ZOrderBitString.C_X / 2)
                    ) % ZOrderBitString.C_X
                elif y_next >= ZOrderBitString.C_Y:
                    # the y-coordinate 'flips', we can account for this
                    # by rotating around x and set y to C_Y - step size = original y
                    y_next = a.y_min
                    # if we overflow the x coordinate wraps around
                    x_next = (
                        x_next + math.floor(ZOrderBitString.C_X / 2)
                    ) % ZOrderBitString.C_X

                # clear bottom bits of the x coordinate, might be messed up after wrapping around
                bl = len(bin(x_next)[2:]) - a.x_precision
                if bl > 0:
                    x_next = (x_next >> bl) << bl

                q.append(
                    ZOrderBitString(
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
    while bit_string_idx < len(intersecting_areas):
        bit_string = intersecting_areas[bit_string_idx]
        # increase idx for the next iteration
        bit_string_idx += 1

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
        results.append(bit_string)

        if plot:
            x, y = ZOrderBitString.from_bit_string(
                bit_string=bit_string
            ).to_shapely_area().exterior.xy

            plt.plot(x, y, color="g", linewidth=5)

    return results


def voxel_bounds_to_2d_wkt_polygon(bounds: Tuple[GeodeticCoordinate, GeodeticCoordinate]) -> str:
    voxel_min, voxel_max = bounds

    return f"((" +\
        f"{voxel_min.longitude} {voxel_min.latitude}" + f"," +\
        (
            f"{voxel_min.longitude + (voxel_max.longitude - voxel_min.longitude) / 2} {voxel_min.latitude},"
            if (voxel_max.longitude - voxel_min.longitude >= 180)
            else f""
        ) +\
        f"{voxel_max.longitude} {voxel_min.latitude}" + f"," +\
        (
            f"{voxel_max.longitude} {voxel_min.latitude + (voxel_max.latitude - voxel_min.latitude) / 2},"
            if (voxel_max.latitude - voxel_min.latitude >= 180)
            else f""
        ) +\
        f"{voxel_max.longitude} {voxel_max.latitude}" + f"," +\
        (
            f"{voxel_min.longitude + (voxel_max.longitude - voxel_min.longitude) / 2} {voxel_max.latitude},"
            if (voxel_max.longitude - voxel_min.longitude >= 180)
            else f""
        ) +\
        f"{voxel_min.longitude} {voxel_max.latitude}" + f"," +\
        (
            f"{voxel_min.longitude} {voxel_min.latitude + (voxel_max.latitude - voxel_min.latitude) / 2},"
            if (voxel_max.latitude - voxel_min.latitude >= 180)
            else f""
        ) +\
        f"{voxel_min.longitude} {voxel_min.latitude}" +\
        f"))"

# random example:
# 66 bits: 100101100010100111010011111100110110000110111110011011010101000001
# 57 bits: 100101100010100111010011111100110110000110111110011011010
# print(ZOrderBitString.msb_binary_string_to_int("0111", 5))


# l = sphere_to_2d_binary_strings(
#     GeodeticCoordinate(
#         # latitude=85.467216,
#         latitude=5.467216,
#         longitude=15.547025,
#         altitude=0
#     ),
#     10
# )
l = sphere_to_coarse_2d_binary_strings(
    center=GeodeticCoordinate(
        # latitude=85.467216,
        longitude=8.5389201,
        latitude=47.3771551,
        altitude=0
    ),
    radius_m=10,
    f_grow=1,
    f_min=0,
    plot=True
)

print("number of bit strings: ", len(l))

p: MultiPolygon = from_wkt("MULTIPOLYGON(" + ",".join([
    voxel_bounds_to_2d_wkt_polygon(
        ZOrderBitString.from_bit_string(b).to_voxel_bounds()
    )
    for b in l
]) + ")")

# for poly in p.geoms:
#     x, y = poly.exterior.xy
#     plt.plot(x, y, c="b")

plt.show()

f = open("./out.txt", "w")

f.write("MULTIPOLYGON(" + ",".join([
    b
    # voxel_bounds_to_2d_wkt_polygon(
    #     ZOrderBitString.from_bit_string(b).to_voxel_bounds()
    # )
    for b in l
]) + ")")

f.close()
