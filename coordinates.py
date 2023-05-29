from __future__ import annotations
from typing import List, Tuple, Union, Dict, Set
import math
from collections import deque
from geopy import distance
from shapely import box, Point, Polygon, GEOSException

SEMI_MAJOR_AXIS_A_M = 6378137.0
SEMI_MINOR_AXIS_B_M = 6356752.3142


class EarthCentricCartesianCoordinate:

    def __init__(
            self,
            x: float,
            y: float,
            z: float
    ) -> None:

        self.x = x
        self.y = y
        self.z = z

    def __str__(self) -> str:
        return f"""
x: {self.x}
y: {self.y}
z: {self.z}
      """.strip()

    def distance(self, other: EarthCentricCartesianCoordinate) -> float:
        # pythagoras
        return math.sqrt(
            ((self.x - other.x) ** 2) +
            ((self.y - other.y) ** 2) +
            ((self.z - other.z) ** 2)
        )


class GeodeticCoordinate:

    def __init__(
            self,
            longitude: float,
            latitude: float,
            altitude: float
    ) -> None:
        assert longitude >= -180 and longitude <= 180
        assert latitude >= -90 and latitude <= 90
        assert altitude >= ZOrderBitString.D and \
            altitude <= ZOrderBitString.C_Z * ZOrderBitString.U

        self.longitude = longitude
        self.latitude = latitude
        self.altitude = altitude

    def __str__(self) -> str:
        return f"""
Longitude: {self.longitude}
Latitude: {self.latitude}
Altitude: {self.altitude}
      """.strip()

    def distance_on_geoid(self, other: GeodeticCoordinate) -> float:
        return distance.distance((self.latitude, self.longitude), (other.latitude, other.longitude)).m

    def eucledian_distance(self, other: GeodeticCoordinate) -> float:
        return self.to_cartesian().distance(other.to_cartesian())

    def to_cartesian(self) -> EarthCentricCartesianCoordinate:
        # convert latitude and longitude to radiants

        # full circle = 360 = 2 pi
        phi = self.latitude / 360 * 2 * math.pi
        # half circle = 180 = pi
        lmbda = self.longitude / 180 * math.pi

        h = self.altitude

        # https://en.wikipedia.org/wiki/Geodetic_coordinates#Conversion
        # https://en.wikipedia.org/wiki/Geographic_coordinate_conversion#From_geodetic_to_ECEF_coordinates
        N = (SEMI_MAJOR_AXIS_A_M ** 2) / math.sqrt(
            (SEMI_MAJOR_AXIS_A_M ** 2) * (math.cos(phi) ** 2) +
            (SEMI_MINOR_AXIS_B_M ** 2) * (math.sin(phi) ** 2)
        )

        X = (N + h) * math.cos(phi) * math.cos(lmbda)
        Y = (N + h) * math.cos(phi) * math.sin(lmbda)
        Z = (
            (
                (SEMI_MINOR_AXIS_B_M ** 2) /
                (SEMI_MAJOR_AXIS_A_M ** 2)
            ) * N
            + h
        ) * math.sin(phi)

        return EarthCentricCartesianCoordinate(
            x=X,
            y=Y,
            z=Z
        )


class ZOrderBitString:
    X_BITS = 26
    Y_BITS = 25
    Z_BITS = 15
    U = 1

    MAX_BITSTRING_LENGTH = X_BITS + Y_BITS + Z_BITS

    # the maximum discretized longitude, latitude and altitude values
    C_X = (1 << X_BITS) - 1
    C_Y = (1 << Y_BITS) - 1
    C_Z = (1 << Z_BITS) - 1

    # the maximum depth
    D = -10000

    def __init__(
            self,
            x_min: int,
            x_precision: int,
            y_min: int,
            y_precision: int,
            z_min: int,
            z_precision: int
    ) -> None:

        assert x_min >= 0 and x_min <= ZOrderBitString.C_X
        assert y_min >= 0 and y_min <= ZOrderBitString.C_Y
        assert z_min >= 0 and z_min <= ZOrderBitString.C_Z

        assert x_precision >= 0 and x_precision <= ZOrderBitString.X_BITS
        assert y_precision >= 0 and y_precision <= ZOrderBitString.Y_BITS
        assert z_precision >= 0 and z_precision <= ZOrderBitString.Z_BITS

        # the x_precision and y_precision can be equal or x_precision one larger
        assert (x_precision == y_precision) or (x_precision == y_precision + 1)
        # z_precision can only be > 0 if x_precision and y_precision are maxed out
        assert z_precision == 0 or (
            x_precision == ZOrderBitString.X_BITS or
            y_precision == ZOrderBitString.Y_BITS
        )

        # ensure that the lowest ZOrderBitString.X_BITS - x_precision bits are cleared
        assert all(bit == '0' for bit in bin(x_min)[2:][x_precision:])
        assert all(bit == '0' for bit in bin(y_min)[2:][y_precision:])
        assert all(bit == '0' for bit in bin(z_min)[2:][z_precision:])

        # more efficient variant:
        # assert (x_min % (1 << (ZOrderBitString.X_BITS - x_precision))) == 0

        self.x_min = x_min
        self.x_precision = x_precision

        self.y_min = y_min
        self.y_precision = y_precision

        self.z_min = z_min
        self.z_precision = z_precision

        self.x_max = x_min + \
            (1 << (ZOrderBitString.X_BITS - x_precision))
        self.y_max = y_min + \
            (1 << (ZOrderBitString.Y_BITS - y_precision))
        self.z_max = z_min + \
            (1 << (ZOrderBitString.Z_BITS - z_precision))

    def __str__(self) -> str:
        return f"""
X: [{self.x_min}, {self.x_max})
Y: [{self.y_min}, {self.y_max})
Z: [{self.z_min}, {self.z_max})
        """.strip()

    @staticmethod
    def discretized_values_to_bit_string(
        x: int,
        x_precision: int,
        y: int,
        y_precision: int,
        z: int,
        z_precision: int
    ) -> str:
        assert x <= ZOrderBitString.C_X
        assert y <= ZOrderBitString.C_Y
        assert z <= ZOrderBitString.C_Z

        x_bits = bin(x)[2:].rjust(ZOrderBitString.X_BITS, '0')[:x_precision]
        y_bits = bin(y)[2:].rjust(ZOrderBitString.Y_BITS, '0')[:y_precision]

        interleaved_xy_bits = "".join(
            i + j for i, j in zip(
                x_bits,
                y_bits
            )
        ) + (x_bits[-1:] if x_precision > y_precision else '')  # X_BITS = Y_BITS OR X_BITS = Y_BITS + 1

        return interleaved_xy_bits + bin(z)[2:].rjust(ZOrderBitString.Z_BITS, '0')[:z_precision]

    def to_bit_string(self) -> str:
        return ZOrderBitString.discretized_values_to_bit_string(
            self.x_min,
            self.x_precision,
            self.y_min,
            self.y_precision,
            self.z_min,
            self.z_precision
        )

    @staticmethod
    def msb_bit_string_to_int(bit_string: str, max_bits: int) -> int:
        assert len(bit_string) <= max_bits

        a = 0
        for i, bit in enumerate(bit_string):
            power = max_bits - 1 - i
            a += (1 << power) * (bit == '1')

        return a

    @staticmethod
    def undiscretize(x: int, y: int, z: int) -> GeodeticCoordinate:
        return GeodeticCoordinate(
            longitude=(x * 360 / (ZOrderBitString.C_X + 1) - 180),
            latitude=(y * 180 / (ZOrderBitString.C_Y + 1) - 90),
            altitude=(ZOrderBitString.D + z * ZOrderBitString.U)
        )

    def to_coordinate(self) -> GeodeticCoordinate:
        return ZOrderBitString.undiscretize(
            self.x_min,
            self.y_min,
            self.z_min
        )

    def to_voxel_bounds(self) -> Tuple[GeodeticCoordinate, GeodeticCoordinate]:
        return (
            ZOrderBitString.undiscretize(
                self.x_min,
                self.y_min,
                self.z_min
            ),
            ZOrderBitString.undiscretize(
                self.x_max,
                self.y_max,
                self.z_max
            )
        )

    def to_shapely_area(self) -> Polygon:
        min_point, max_point = self.to_voxel_bounds()

        return box(
            xmin=min_point.longitude,
            ymin=min_point.latitude,
            xmax=max_point.longitude,
            ymax=max_point.latitude
        )

    def to_voxel(self) -> List[GeodeticCoordinate]:
        return [
            ZOrderBitString.undiscretize(
                x,
                y,
                z
            )
            for x in [self.x_min, self.x_max]
            for y in [self.y_min, self.y_max]
            for z in [self.z_min, self.z_max]
        ]

    def eucledian_distance_voxel_point(self, point: Union[GeodeticCoordinate, EarthCentricCartesianCoordinate]) -> float:
        v1 = self.to_voxel()

        if isinstance(point, GeodeticCoordinate):
            return min(
                p.eucledian_distance(point)
                for p in v1
            )
        elif isinstance(point, EarthCentricCartesianCoordinate):
            return min(
                p.to_cartesian().distance(point)
                for p in v1
            )
        else:
            raise Exception(f"Unsupported point type '{type(point)}'")

    def eucledian_distance_voxel_voxel(self, other: ZOrderBitString) -> float:
        v1 = self.to_voxel()
        v2 = other.to_voxel()

        return min(
            p.eucledian_distance(q)
            for p in v1
            for q in v2
        )

    @staticmethod
    def from_compact_bit_string(
        compact_bit_string: int
    ) -> ZOrderBitString:
        return ZOrderBitString.from_bit_string(
            bin(compact_bit_string)[2:]
        )

    @staticmethod
    def from_bit_string(
        bit_string: str
    ) -> ZOrderBitString:
        assert len(bit_string) <= ZOrderBitString.MAX_BITSTRING_LENGTH

        # B_{x} + B_{y}
        xyBits = ZOrderBitString.X_BITS + ZOrderBitString.Y_BITS

        # get every other coordinate starting at 0 up to 2 * B_{x,y}
        x_coordinate = bit_string[0:xyBits:2]
        # get every other coordinate starting at 1 up to 2 * B_{x,y}
        y_coordinate = bit_string[1:xyBits:2]
        # get all bits after 2 * B_{x,y}
        z_coordinate = bit_string[xyBits:]

        x_min = ZOrderBitString.msb_bit_string_to_int(
            x_coordinate,
            ZOrderBitString.X_BITS
        )

        y_min = ZOrderBitString.msb_bit_string_to_int(
            y_coordinate,
            ZOrderBitString.Y_BITS
        )

        z_min = ZOrderBitString.msb_bit_string_to_int(
            z_coordinate,
            ZOrderBitString.Z_BITS
        )

        return ZOrderBitString(
            x_min=x_min,
            x_precision=len(x_coordinate),
            y_min=y_min,
            y_precision=len(y_coordinate),
            z_min=z_min,
            z_precision=len(z_coordinate),
        )

    @staticmethod
    def from_coordinate(
        coordinate: GeodeticCoordinate
    ) -> ZOrderBitString:
        assert coordinate.longitude >= -180
        assert coordinate.longitude <= 180

        x = math.floor(
            (
                (coordinate.longitude + 180) / 360
            ) * ZOrderBitString.C_X
        )

        assert coordinate.latitude >= -90
        assert coordinate.latitude <= 90

        y = math.floor(
            (
                (coordinate.latitude + 90) / 180
            ) * ZOrderBitString.C_Y
        )

        z = math.floor(
            (coordinate.altitude - ZOrderBitString.D) / ZOrderBitString.U
        )

        return ZOrderBitString(
            x_min=x,
            x_precision=ZOrderBitString.X_BITS,
            y_min=y,
            y_precision=ZOrderBitString.Y_BITS,
            z_min=z,
            z_precision=ZOrderBitString.Z_BITS
        )


def grow_area(initial_area: ZOrderBitString, area: float):
    # while the area of the object is larger than a fraction of the grid's area
    # decrease the grid's size
    while (
        initial_area.x_precision > 1 and
        ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        ).to_shapely_area().area < area
    ):

        # grow area by removing one bit
        initial_area = ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        )

    return initial_area


def polygons_to_2d_bit_strings(
        polygons: List[Polygon],
        f_grow: float,
        f_min=0.0
) -> List[str]:
    """
    Computes a set of 2D bit strings from a given set of polygons.
    `f_grow` and `f_min` are parameters influencing the accuracy
    of the approximation.

    The algorithm first computes the smallest voxel corresponding
    to a random polygon vertex. It then grows this voxel's until
    it's 2D shadow covers `f_grow` of the polygon's area.

    In a next step, a BFS among the voxel's neighbors is performed
    and the neighboring voxels are checked for intersection with
    the polygon. If the intersection's area is at
    least `f_min` of the voxel's area, it is taken and otherwise
    it is ignored. With `f_min = 0`, the polygon is over-approximated,
    with `f_min < 0` it is under-approximated.

    After the BFS, neighboring voxels intersecting the polygon are
    merged and only their parent bit string is returned.
    Redundant bit strings are omitted (e.g. ones where the result
    also contains a prefix of them).

    The level of the approximation's accuracy is determined by `f_grow`.
    By setting `f_grow = 0`, the best possible approximation is computed,
    resulting in more bit strings.

    Parameters
    ----------
    :param polygons: The list of polygon to turn into bit strings
    :param f_grow: The fraction of a polygons area which should be used for the voxel size
    :param f_min: The minimum fraction of the 2D shadow of a voxel that has to intersect
        a polygon for the bit string to be considered.
    :returns: A list of 2D bit strings approximating the circle
    """

    intersecting_areas_all_polygons: Set[str] = set()

    initial_areas = [
        ZOrderBitString.from_bit_string(
            ZOrderBitString.from_coordinate(
                GeodeticCoordinate(
                    longitude=polygon.exterior.coords[0][0],
                    latitude=polygon.exterior.coords[0][1],
                    altitude=0
                )
            ).to_bit_string()
            # remove z-bits
            [:(ZOrderBitString.X_BITS + ZOrderBitString.Y_BITS)]
        )
        for polygon in polygons
    ]

    for polygon, initial_area in zip(polygons, initial_areas):
        # this will be the list of bitstrings of the chosen size
        intersecting_areas: Set[str] = set()

        # perform the BFS
        visited: Dict[str, bool] = {}
        q = deque()
        q.append(
            grow_area(
                initial_area=initial_area,
                # area=multi_polygon.area,
                area=polygon.area * f_grow
            )
        )

        while len(q) > 0:
            voxel: ZOrderBitString = q.popleft()
            bit_string = voxel.to_bit_string()

            if bit_string in visited:
                continue

            # mark as visited
            visited[bit_string] = True

            voxel_shadow = voxel.to_shapely_area()

            # check for intersection. always take the first area
            try:
                if len(intersecting_areas) > 0 and not (
                    voxel_shadow.intersection(
                        polygon
                    ).area > f_min * voxel_shadow.area
                ):

                    continue
            except GEOSException:
                if len(intersecting_areas) > 0 and not (
                   voxel_shadow.intersection(
                       polygon.buffer(0)
                   ).area > f_min * voxel_shadow.area
                   ):

                    continue

            # add to intersection list
            intersecting_areas.add(bit_string)

            # visit neighbors of a
            for dx in [-1, 0, 1]:
                for dy in [-1, 0, 1]:

                    # compute neighbor coordinates
                    y_next = (
                        voxel.y_min + dy *
                        (1 << (ZOrderBitString.Y_BITS - voxel.y_precision))
                    )

                    x_next = (
                        voxel.x_min + dx *
                        (1 << (ZOrderBitString.X_BITS - voxel.x_precision))
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
                        y_next = voxel.y_min
                        # if we overflow the x coordinate wraps around
                        x_next = (
                            x_next + math.floor(ZOrderBitString.C_X / 2)
                        ) % ZOrderBitString.C_X

                    # clear bottom bits of the x coordinate, might be messed up after wrapping around
                    bl = len(bin(x_next)[2:]) - voxel.x_precision
                    if bl > 0:
                        x_next = (x_next >> bl) << bl

                    q.append(
                        ZOrderBitString(
                            x_min=x_next,
                            x_precision=voxel.x_precision,
                            y_min=y_next,
                            y_precision=voxel.y_precision,
                            # z will stay the same
                            z_min=voxel.z_min,
                            z_precision=voxel.z_precision,
                        )
                    )

        # append `intersecting_areas` to list for all polygons
        intersecting_areas_all_polygons = intersecting_areas_all_polygons.union(
            intersecting_areas
        )

    # after computing the intersecting voxels, merge them and remove redundant ones
    results: List[str] = []

    # transform set to list
    intersecting_areas_all_polygons_list: List[str] = list(
        intersecting_areas_all_polygons
    )

    bit_string_idx = 0
    while bit_string_idx < len(intersecting_areas_all_polygons_list):
        bit_string = intersecting_areas_all_polygons_list[bit_string_idx]
        # increase idx for the next iteration
        bit_string_idx += 1

        # check if this bit string is redundant, i.e. a shorter prefix is also
        # part of
        skip = False
        # iterate over all prefixes of that bitstring from largest/shortest to smallest/longest
        for i in range(1, len(bit_string)):
            # check if any of its prefixes (larger areas) is also part of intersecting_areas_all_polygons_list
            if bit_string[:i] in intersecting_areas_all_polygons_list:
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

        if bit_string_neighbor in intersecting_areas_all_polygons_list:
            # yes it can. ignore current bit_string by skipping (continue)
            # if the neighbor is visited afterwards it will be skipped because
            # the list contains a prefix of it

            # add parent at the end of the list to make sure duplicate test is performed with parent again
            intersecting_areas_all_polygons_list.append(bit_string[:-1])

        # from this point on bit_string is sucessfully taken
        results.append(bit_string)

    return results


def sphere_to_polygon(
        center: GeodeticCoordinate,
        radius_m: float,
        quad_segs=16
) -> Polygon:
    """
    Approximates a circle defined by geodetic coordinates and a radius
    in meters using a shapely polygon in the eucledian geodetic space.

    First `radius_m` are walked in a few directions (bearing) from
    the center, then the eucledian distances to these points
    using in the eucledian geodetic space are computed and the
    maximum is used to approximate the circle.

    Using this radius, the circle is then approximated as a
    `4 * quad_segs` sided polygon.

    Parameters
    ----------
    :param center: The center of the circle
    :param radius_m: The radius of the circle in meters
    :returns: A polygon approximation of the circle
    """

    # approximate circle, accuracy is slightly less important for correctness
    # as this is computed by the client
    # longitudes per meter is always greater than latitude per meter

    # walk `radius_m` in a few directions (bearing) from the center
    positions = (
        distance.distance(
            meters=radius_m
        ).destination(
            (center.latitude, center.longitude),
            bearing=bearing
        )
        # a range of degrees
        for bearing in [45, 90, 135, 225, 270]
    )

    # then compute the eucledian distance using the geodetic coordinates
    # note that the resulting unit is meaningless but for small `radius_m`
    # it will locally be a approximately a constant factor off

    # use the maximum of these distances as the radius in the eucledian
    # longitude / latitude space
    radius_lon_lat = math.sqrt(
        max(
            (p.longitude - center.longitude) ** 2 +
            (p.latitude - center.latitude) ** 2
            for p in positions
        )
    )
    # and use this as a radius for a circle in that space. approximate this
    # circle using a `quad_segs` * 4 sided polygon
    circle: Polygon = Point(center.longitude, center.latitude).buffer(
        radius_lon_lat,
        quad_segs=quad_segs
    )

    return circle


def smallest_enclosing_z_bit_string(
        altitude_min: float,
        altitude_max: float
) -> str:
    """
    Returns the single longest / most precise bit string encompassing both,
    `altitude_min` and `altitude_max`. In contrast to
    `polygons_to_2d_bit_strings`. Since it only returns
    a single bit string it is much more likely to use a shorter / less
    precise bit string than `polygons_to_2d_bit_strings` but
    results in a sparser tree. Under the assumption that the altitude
    is rather sparse this seems to be a good tradeoff.

    Parameters
    ----------
    :param altitude_min: The minimum altitude that should be covered, [D, altitude_max)
    :param altitude_max: The maximum altitude that should be covered, (altitude_min, H]
    :returns: The most precise bit string encompassing the two altitude values
    """

    discretized_z_min = bin(
        math.floor(
            (altitude_min - ZOrderBitString.D) / ZOrderBitString.U
        )
    )[2:].rjust(ZOrderBitString.Z_BITS, "0")

    discretized_z_max = bin(
        math.floor(
            (altitude_max - ZOrderBitString.D) / ZOrderBitString.U
        )
    )[2:].rjust(ZOrderBitString.Z_BITS, "0")

    bit_string = ""
    for b1, b2 in zip(discretized_z_min, discretized_z_max):
        if b1 != b2:
            break

        bit_string += b1

    # use max, bit string can be longer
    return bit_string


def extruded_polygons_to_bit_strings(
        polygons: List[Polygon],
        altitude_min: float,
        altitude_max: float,
        f_grow: float
) -> Tuple[List[str], List[str], str]:
    """
    Returns all bit strings to cover the given extruded polygon.

    Parameters
    ----------
    :param polygons: A list of polygons that should be mapped to voxels
    :param altitude_min: The lower altitude bound for the extruded polygon, [D, altitude_max)
    :param altitude_max: The upper altitude bound for the extruded polygon, (altitude_min, H]
    :param f_grow: The fraction of a polygons area which should be used for the voxel size
    :returns: A set of bit strings 
    """

    XY_BITS = ZOrderBitString.X_BITS + ZOrderBitString.Y_BITS

    z_bit_string = smallest_enclosing_z_bit_string(
        altitude_min=altitude_min,
        altitude_max=altitude_max
    )

    # if z_bit_string != '':
    #     # not full height, f_grow > 0 does not make sense since
    #     # we need to go to the full depth anyway
    #     f_grow = 0

    xy_bit_strings = polygons_to_2d_bit_strings(
        polygons=polygons,
        f_grow=f_grow,
        # always over-approximate
        f_min=0
    )

    # the full height has to be covered
    if z_bit_string == '':
        return xy_bit_strings, xy_bit_strings, ''

    results: List[str] = []

    # when we don't use tuples we have to compute all possible
    # substrings up to a length of 51 for the xy_bit_strings
    # and then append the z_bit_string
    for xy_bit_string in xy_bit_strings:
        fill_length = XY_BITS - len(xy_bit_string)

        assert fill_length >= 0

        # check if xy_bit_string already has the full length
        if fill_length == 0:
            results.append(
                xy_bit_string + z_bit_string
            )
            continue

        # if not, several bit strings have to be generated
        max_int = int(fill_length * "1", 2)

        for i in range(max_int + 1):
            results.append(
                xy_bit_string +
                bin(i)[2:].ljust(fill_length, "0") +
                z_bit_string
            )

    return results, xy_bit_strings, z_bit_string


def extruded_polygons_to_bit_string_counts(
        polygons: List[Polygon],
        altitude_min: float,
        altitude_max: float,
        f_grow: float
) -> Tuple[int, int]:
    """
    Returns all bit strings to cover the given extruded polygon.

    Parameters
    ----------
    :param polygons: A list of polygons that should be mapped to voxels
    :param altitude_min: The lower altitude bound for the extruded polygon, [D, altitude_max)
    :param altitude_max: The upper altitude bound for the extruded polygon, (altitude_min, H]
    :param f_grow: The fraction of a polygons area which should be used for the voxel size
    """

    XY_BITS = ZOrderBitString.X_BITS + ZOrderBitString.Y_BITS

    z_bit_string = smallest_enclosing_z_bit_string(
        altitude_min=altitude_min,
        altitude_max=altitude_max
    )

    # if z_bit_string != '':
    #     # not full height, f_grow > 0 does not make sense since
    #     # we need to go to the full depth anyway
    #     f_grow = 0

    xy_bit_strings = polygons_to_2d_bit_strings(
        polygons=polygons,
        f_grow=f_grow,
        # always over-approximate
        f_min=0
    )

    # the full height has to be covered
    if z_bit_string == '':
        return len(xy_bit_strings), len(xy_bit_strings)

    results = 0

    # when we don't use tuples we have to compute all possible
    # substrings up to a length of 51 for the xy_bit_strings
    # and then append the z_bit_string
    for xy_bit_string in xy_bit_strings:
        fill_length = XY_BITS - len(xy_bit_string)

        assert fill_length >= 0

        # check if xy_bit_string already has the full length
        if fill_length == 0:
            results += 0
            continue

        # if not, several bit strings have to be generated
        max_int = int(fill_length * "1", 2)

        results += max_int + 1

    return results, len(xy_bit_strings)


def sphere_to_coarse_2d_binary_strings(
        center: GeodeticCoordinate,
        radius_m: float,
        f_grow: float
):
    polygon = sphere_to_polygon(center, radius_m=radius_m)

    return polygons_to_2d_bit_strings(
        polygons=[polygon],
        f_grow=f_grow,
        # always over-approximate
        f_min=0
    )
