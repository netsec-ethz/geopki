from __future__ import annotations
from typing import List, Tuple, Union, Dict, Set
import math
from collections import deque
from geopy import distance
from shapely import box, Point, Polygon, GEOSException

SEMI_MAJOR_AXIS_A_M = 6378137.0
"The semi major axis of the WGS84 ellipsoid model ('radius' at the equator)."

SEMI_MINOR_AXIS_B_M = 6356752.3142
"The semi minor axis of the WGS84 ellipsoid model ('radius' at the poles)."


class EarthCentricCartesianCoordinate:
    """
    Representation of a earth-centered, earth fixed (ECEF) coordinate.
    ECEF is a cartesian coordinate system which allows distances
    to be computed using the pythagorean theorem.
    """

    def __init__(
            self,
            x: float,
            y: float,
            z: float
    ):
        """
        Creates a new instance of an EEC coordinate

        Parameters
        ----------
        :param x: The x coordinate in the EEC coordinate system
        :param y: The y coordinate in the EEC coordinate system
        :param z: The z coordinate in the EEC coordinate system
        :returns: An `EarthCentricCartesianCoordinate` instance
        """

        self.x = x
        self.y = y
        self.z = z

    def __str__(self) -> str:
        """
        Returns a string representation of the ECEF coordinate
        """
        return (
            f"x: {self.x}"
            f"y: {self.y}"
            f"z: {self.z}"
        )

    def distance(self, other: EarthCentricCartesianCoordinate) -> float:
        """
        Computes the distance to another coordinate using the pythagorean theorem.
        """

        return math.sqrt(
            ((self.x - other.x) ** 2) +
            ((self.y - other.y) ** 2) +
            ((self.z - other.z) ** 2)
        )


class GeodeticCoordinate:
    """
    Representation of a geodetic coordinate.
    """

    def __init__(
            self,
            longitude: float,
            latitude: float,
            altitude: float
    ):
        """
        Creates a new instance of a geodetic coordinate

        Parameters
        ----------
        :param longitude: The longitude, must be in `[-180, 180]`
        :param latitude: The latitude, must be in `[-90, 90]`
        :param altitude: The altitude, must be within `[D, H]`
        :returns: A `GeodeticCoordinate` instance
        """

        assert longitude >= -180 and longitude <= 180
        assert latitude >= -90 and latitude <= 90

        self.longitude = longitude
        self.latitude = latitude
        self.altitude = altitude

    def __str__(self) -> str:
        """
        Returns a string representation of the ECEF coordinate
        """
        return (
            f"Longitude: {self.longitude}"
            f"Latitude: {self.latitude}"
            f"Altitude: {self.altitude}"
        )

    def distance_on_ellipsoid(self, other: GeodeticCoordinate) -> float:
        """
        Computes the 2D distance to another geodetic coordinate on the WGS84 ellipsoid.
        """
        return distance.distance((self.latitude, self.longitude), (other.latitude, other.longitude)).m

    def eucledian_distance(self, other: GeodeticCoordinate) -> float:
        """
        Computes the 3D eucledian distance (straight line) to another geodetic coordinate.
        """
        return self.to_cartesian().distance(other.to_cartesian())

    def to_cartesian(self) -> EarthCentricCartesianCoordinate:
        """
        Returns the corresponding ECEF coordinate
        """

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


class DiscretizedVoxel:
    """
    Representation of a node in the sparse merkle tree (SMT).
    Has an associated bit string tuple, a discretized coordinate and a voxel.
    """

    U = 1
    "The precision of the tree in meters"

    D = -10000
    "The minimum geodetic altitude in meters"

    H = 22767
    "The maximum geodetic altitude in meters"

    X_BITS = math.floor(math.log2(2 * SEMI_MAJOR_AXIS_A_M * math.pi / U)) + 1
    """
    The number of bits in used in the discretization of the `x` dimension.
    `X_BITS = 26` for `U = 1`
    """

    Y_BITS = math.floor(math.log2(SEMI_MINOR_AXIS_B_M * math.pi / U)) + 1
    """
    The number of bits in used in the discretization of the `y` dimension.
    `Y_BITS = 25` for `U = 1`
    """

    Z_BITS = math.floor(math.log2((H - D) / U)) + 1
    """
    The number of bits in used in the discretization of the `z` dimension.
    `Z_BITS = 15` for `U = 1`
    """

    # the maximum discretized longitude, latitude and altitude values
    C_X = (1 << X_BITS) - 1
    """
    The maximum value of the discretized `x` coordinate.
    """

    C_Y = (1 << Y_BITS) - 1
    """
    The maximum value of the discretized `y` coordinate.
    """

    C_Z = (1 << Z_BITS) - 1
    """
    The maximum value of the discretized `z` coordinate.
    """

    def __init__(
            self,
            x_min: int,
            x_precision: int,
            y_min: int,
            y_precision: int,
            z_min: int,
            z_precision: int
    ):
        """
        Creates a new `DiscretizedVoxel` instance.
        The following relationships between the arguments must hold:

            - Valid bit string representation with interleaving x and y bits

               `(x_precision == y_precision) or (x_precision == y_precision + 1)`

        Parameters
        ----------
        :param x_min: The smallest x coordinate of the voxel, must be in `[0, C_X]`
        :param x_precision: The number of bits used to encode `x_min`, must be in `[0, X_BITS]`
        :param y_min: The smallest y coordinate of the voxel, must be in `[0, C_Y]`
        :param y_precision: The number of bits used to encode `y_min`, must be in `[0, Y_BITS]`
        :param z_min: The smallest z coordinate of the voxel, must be in `[0, C_Z]`
        :param z_precision: The number of bits used to encode `z_min`, must be in `[0, Z_BITS]`
        :returns: A `DiscretizedVoxel` instance
        """

        assert x_min >= 0 and x_min <= DiscretizedVoxel.C_X
        assert y_min >= 0 and y_min <= DiscretizedVoxel.C_Y
        assert z_min >= 0 and z_min <= DiscretizedVoxel.C_Z

        assert x_precision >= 0 and x_precision <= DiscretizedVoxel.X_BITS
        assert y_precision >= 0 and y_precision <= DiscretizedVoxel.Y_BITS
        assert z_precision >= 0 and z_precision <= DiscretizedVoxel.Z_BITS

        # the x_precision and y_precision can be equal or x_precision one larger
        assert (x_precision == y_precision) or (x_precision == y_precision + 1)

        # ensure that the lowest ZOrderBitString.X_BITS - x_precision bits are cleared
        assert all(bit == '0' for bit in bin(x_min)[2:][x_precision:])
        assert all(bit == '0' for bit in bin(y_min)[2:][y_precision:])
        assert all(bit == '0' for bit in bin(z_min)[2:][z_precision:])

        # more efficient variant:
        # assert (x_min % (1 << (ZOrderBitString.X_BITS - x_precision))) == 0

        self.x_min = x_min
        "The minimum discretized x coordinate that is witin the voxel"
        self.x_precision = x_precision
        "The number of bits used to encode `x_min`. Determines the size of the voxel."

        self.y_min = y_min
        "The minimum discretized y coordinate that is witin the voxel"
        self.y_precision = y_precision
        "The number of bits used to encode `y_min`. Determines the size of the voxel."

        self.z_min = z_min
        "The minimum discretized z coordinate that is witin the voxel"
        self.z_precision = z_precision
        "The number of bits used to encode `z_min`. Determines the size of the voxel."

    def get_x_bit_string(self) -> str:
        "The bit string encoding the `x_min` value, i.e. the `y_precision` MSBs"
        return bin(self.x_min)[2:].rjust(
            DiscretizedVoxel.X_BITS,
            '0'
        )[:self.x_precision]

    def get_y_bit_string(self) -> str:
        "The bit string encoding the `y_min` value, i.e. the `y_precision` MSBs"
        return bin(self.y_min)[2:].rjust(
            DiscretizedVoxel.Y_BITS,
            '0'
        )[:self.y_precision]

    def get_z_bit_string(self) -> str:
        "The bit string encoding the `z_min` value, i.e. the `y_precision` MSBs"
        return bin(self.z_min)[2:].rjust(
            DiscretizedVoxel.Z_BITS,
            '0'
        )[:self.z_precision]

    def get_x_max(self) -> int:
        "The smallest discretized `x` coordinate that is no longer in the voxel"
        return DiscretizedVoxel.bit_string_to_max_int(
            self.get_x_bit_string(),
            DiscretizedVoxel.X_BITS
        ) + 1

    def get_y_max(self) -> int:
        "The smallest discretized `y` coordinate that is no longer in the voxel"
        return DiscretizedVoxel.bit_string_to_max_int(
            self.get_y_bit_string(),
            DiscretizedVoxel.Y_BITS
        ) + 1

    def get_z_max(self) -> int:
        "The smallest discretized `z` coordinate that is no longer in the voxel"
        return DiscretizedVoxel.bit_string_to_max_int(
            self.get_z_bit_string(),
            DiscretizedVoxel.Z_BITS
        ) + 1

    def __str__(self) -> str:
        """
        Returns a string representation of the discretized coordinate
        """
        return (
            f"X: [{self.x_min}, {self.get_x_max() + 1})\n"
            f"Y: [{self.y_min}, {self.get_y_max() + 1})\n"
            f"Z: [{self.z_min}, {self.get_z_max() + 1})"
        )

    def to_bit_string_tuple(self) -> Tuple[str, str]:
        """
        Returns the bit string representation of the `x` and `y` coordinate and the
        bit representation of the `z` coordinate.
        """
        return DiscretizedVoxel.discretized_values_to_bit_string_tuple(
            self.x_min,
            self.x_precision,
            self.y_min,
            self.y_precision,
            self.z_min,
            self.z_precision
        )

    def to_geodetic_coordinate(self) -> GeodeticCoordinate:
        """
        Returns the geodetic coordinate corresponding to the point with the
        smallest longitude, latitude and altitude.
        """
        return DiscretizedVoxel.undiscretize(
            self.x_min,
            self.y_min,
            self.z_min
        )

    def to_voxel_bounds(self) -> Tuple[GeodeticCoordinate, GeodeticCoordinate]:
        """
        Returns the geodetic coordinates, one corresponding to the points with the
        smallest and largest longitude, latitude and altitude defining the voxel.
        The first value in the tuple is the one with the smallest and the second
        the one with the largest geodetic coordinates.
        """
        return (
            DiscretizedVoxel.undiscretize(
                self.x_min,
                self.y_min,
                self.z_min
            ),
            DiscretizedVoxel.undiscretize(
                self.get_x_max(),
                self.get_y_max(),
                self.get_z_max()
            )
        )

    def to_shapely_area(self) -> Polygon:
        """
        Returns a two dimensional shapely polygon of the voxel's projection
        to the earth's surface in geodetic coordinates.
        """
        min_point, max_point = self.to_voxel_bounds()

        return box(
            xmin=min_point.longitude,
            ymin=min_point.latitude,
            xmax=max_point.longitude,
            ymax=max_point.latitude
        )

    def to_voxel(self) -> List[GeodeticCoordinate]:
        """
        Returns the eight corners of the three dimensional voxel.
        """
        return [
            DiscretizedVoxel.undiscretize(x, y, z)
            for x in [self.x_min, self.get_x_max()]
            for y in [self.y_min, self.get_y_max()]
            for z in [self.z_min, self.get_z_max()]
        ]

    def grow_2d(self, steps=1) -> DiscretizedVoxel:
        """
        Grows (*modifies*) the voxel by decreasing the `x` and `y` precision 'steps' times.
        If the precision of `x` and `y` is equal, the `y` precision is reduced,
        otherwise the `x` precision. Multiplies the covered area by `2 ** steps`
        Throws an exception if it is not possible to grow `steps` times
        """

        # Invariant `(x_precision == y_precision) or (x_precision == y_precision + 1)` must hold

        if self.x_precision + self.y_precision <= steps:
            # cannot grow further, one bit must be left in the end
            raise Exception(f"Cannot grow further in 2D, only one bit left")
        elif self.x_precision == self.y_precision:
            # start with the y bit, if odd clear one more y bit
            x_bits_to_clear = math.floor(steps / 2)
            y_bits_to_clear = math.ceil(steps / 2)
        elif self.x_precision == self.y_precision + 1:
            # start with the x bit, if odd clear one more x bit
            x_bits_to_clear = math.ceil(steps / 2)
            y_bits_to_clear = math.floor(steps / 2)
        else:
            raise Exception(
                f"(x_precision == y_precision) or (x_precision == y_precision + 1) invariant violated"
            )

        x_bit_mask = (
            # flip all bits, should be zeros not ones for clearing
            ~(
                # create a set of `x_bits_to_clear` ones
                ~(-1 << x_bits_to_clear) <<
                # and shift them to a certain position
                (DiscretizedVoxel.X_BITS - self.x_precision)
            )
        )

        y_bit_mask = (
            # flip all bits, should be zeros not ones for clearing
            ~(
                # create a set of `y_bits_to_clear` ones
                ~(-1 << y_bits_to_clear) <<
                # and shift them to a certain position
                (DiscretizedVoxel.Y_BITS - self.y_precision)
            )
        )

        # clear the last few x bits
        self.x_min = self.x_min & x_bit_mask
        # reduce the x precision
        self.x_precision -= x_bits_to_clear

        # clear the last few y bits
        self.y_min = self.y_min & y_bit_mask
        # reduce the y precision
        self.y_precision -= y_bits_to_clear

        return self

    def grow_z(self, steps=1):
        """
        Grows (*modifies*) the voxel by decreasing the `z` precision `steps` times.
        Throws an exception if the it is not possible to grow `z` times.
        Multiplies the covered altitude by `2 ** steps.`
        """

        if self.z_precision <= steps:
            # cannot grow further
            raise Exception(
                f"Cannot grow further in the altitude, only one bit left"
            )
        else:
            z_bit_mask = (
                # flip all bits, should be zeros not ones for clearing
                ~(
                    # create a set of `steps` ones
                    ~(-1 << steps) <<
                    # and shift them to a certain position
                    (DiscretizedVoxel.Z_BITS - self.z_precision)
                )
            )

            # clear the last few z bits
            self.z_min = self.z_min & z_bit_mask
            # reduce the z precision
            self.z_precision -= steps

            return self

    def grow_2d_to_area(
            self,
            max_area: float
    ) -> DiscretizedVoxel:
        """
        Grows (*modifies*) the voxel by removing bits from the 2d bit string until the voxel's
        shadow projected to the earth's surface (`.to_shapely_area()`) would be greater than
        `max_area` if another bit was removed.
        The area units are not meaningful as it is the result of computing an area using geodetic
        coordinates and treating them as certesian coordinates. Still this function can be useful
        for approximating an area.

        Parameters
        ----------
        :param max_area: An upper bound for the area the voxel should have after growing.
        """

        current_area = self.to_shapely_area().area
        grow_steps = math.floor(math.log2(max_area / current_area))

        # no shrinking
        if grow_steps < 0:
            return self

        return self.grow_2d(grow_steps)

    def grow_z_to_length(
            self,
            altitude_max_range: float
    ):
        """
        Grows (*modifies*) the voxel by removing bits from the z bit string until the voxel's
        altitude would be greater than `max_altitude_range` if another bit was removed.

        Parameters
        ----------
        :param altitude_max_range: An upper bound for the altitude the voxel should cover after growing.
        """

        current_altitude_range = self.get_z_max() - self.z_min + 1
        grow_steps = math.floor(
            math.log2(altitude_max_range / current_altitude_range)
        )

        return self.grow_z(grow_steps)

    @staticmethod
    def discretized_values_to_bit_string_tuple(
        x: int,
        x_precision: int,
        y: int,
        y_precision: int,
        z: int,
        z_precision: int
    ) -> Tuple[str, str]:
        """
        Returns the bit string tuple corresponding to a set of
        discretized x, y and z coordinates.

        Parameters
        ----------
        :param x: The discretized `x` coordinate
        :param x_precision: The number of bits encoding the `x` coordinate
        :param y: The discretized `y` coordinate
        :param y_precision: The number of bits encoding the `y` coordinate
        :param z: The discretized `z` coordinate
        :param z_precision: The number of bits encoding the `z` coordinate
        :returns: Bitstring encoding the `x` and `y` coordinates, bitstring encoding the `z` coordinate
        """
        assert x <= DiscretizedVoxel.C_X
        assert y <= DiscretizedVoxel.C_Y
        assert z <= DiscretizedVoxel.C_Z

        x_bits = bin(x)[2:].rjust(
            DiscretizedVoxel.X_BITS,
            '0'
        )[:x_precision]

        y_bits = bin(y)[2:].rjust(
            DiscretizedVoxel.Y_BITS,
            '0'
        )[:y_precision]

        interleaved_xy_bits = "".join(
            i + j for i, j in zip(
                x_bits,
                y_bits
            )
        ) + (x_bits[-1:] if x_precision > y_precision else '')  # X_BITS = Y_BITS OR X_BITS = Y_BITS + 1

        return interleaved_xy_bits, bin(z)[2:].rjust(DiscretizedVoxel.Z_BITS, '0')[:z_precision]

    @staticmethod
    def bit_string_to_min_int(bit_string: str, max_bits: int) -> int:
        """
        Returns the integer obtained represented by the bit string
        obtained by right padding the given bit string to `max_bits`
        length using zeros.

        Parameters
        ----------
        :param bit_string: The bit string to interpret as an integer
        :param max_bits: The length to which the bit string should be padded
        """
        assert len(bit_string) <= max_bits

        return int(bit_string.ljust(max_bits, "0"), 2)

        # a = 0
        # for i, bit in enumerate(bit_string):
        #     power = max_bits - 1 - i
        #     a += (1 << power) * (bit == '1')

        # return a

    @staticmethod
    def bit_string_to_max_int(bit_string: str, max_bits: int) -> int:
        """
        Returns the integer obtained represented by the bit string
        obtained by right padding the given bit string to `max_bits`
        length using ones.

        Parameters
        ----------
        :param bit_string: The bit string to interpret as an integer
        :param max_bits: The length to which the bit string should be padded
        """
        assert len(bit_string) <= max_bits

        return int(bit_string.ljust(max_bits, "1"), 2)

    @staticmethod
    def undiscretize(x: int, y: int, z: int) -> GeodeticCoordinate:
        """
        Returns the geodetic coordinate corresponding to a set of
        discretized x, y and z coordinates.

        Parameters
        ----------
        :param x: The discretized x coordinate
        :param y: The discretized y coordinate
        :param z: The discretized z coordinate
        """
        return GeodeticCoordinate(
            longitude=(x * 360 / (DiscretizedVoxel.C_X + 1) - 180),
            latitude=(y * 180 / (DiscretizedVoxel.C_Y + 1) - 90),
            altitude=(DiscretizedVoxel.D + z * DiscretizedVoxel.U)
        )

    @staticmethod
    def from_bit_string_tuple(
        bit_string_xy: str,
        bit_string_z: str
    ) -> DiscretizedVoxel:
        """
        Creates an instance from a given bit string encoding the x and y coordinate and
        a second bit string encoding the z coordinate.
        """

        # B_{x} + B_{y}
        XY_BITS = DiscretizedVoxel.X_BITS + DiscretizedVoxel.Y_BITS

        assert (
            len(bit_string_xy) <= XY_BITS
        ) and len(bit_string_z) <= DiscretizedVoxel.Z_BITS

        # get every other coordinate starting at 0 up to 2 * B_{x,y}
        x_coordinate = bit_string_xy[0:XY_BITS:2]
        # get every other coordinate starting at 1 up to 2 * B_{x,y}
        y_coordinate = bit_string_xy[1:XY_BITS:2]
        # get all bits after 2 * B_{x,y}
        z_coordinate = bit_string_z

        x_min = DiscretizedVoxel.bit_string_to_min_int(
            x_coordinate,
            DiscretizedVoxel.X_BITS
        )

        y_min = DiscretizedVoxel.bit_string_to_min_int(
            y_coordinate,
            DiscretizedVoxel.Y_BITS
        )

        z_min = DiscretizedVoxel.bit_string_to_min_int(
            z_coordinate,
            DiscretizedVoxel.Z_BITS
        )

        return DiscretizedVoxel(
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
    ) -> DiscretizedVoxel:
        """
        Creates an instance from a geodetic coordinate
        """

        assert coordinate.longitude >= -180
        assert coordinate.longitude <= 180

        x = math.floor(
            (
                (coordinate.longitude + 180) / 360
            ) * DiscretizedVoxel.C_X
        )

        assert coordinate.latitude >= -90
        assert coordinate.latitude <= 90

        y = math.floor(
            (
                (coordinate.latitude + 90) / 180
            ) * DiscretizedVoxel.C_Y
        )

        z = math.floor(
            (coordinate.altitude - DiscretizedVoxel.D) / DiscretizedVoxel.U
        )

        return DiscretizedVoxel(
            x_min=x,
            x_precision=DiscretizedVoxel.X_BITS,
            y_min=y,
            y_precision=DiscretizedVoxel.Y_BITS,
            z_min=z,
            z_precision=DiscretizedVoxel.Z_BITS
        )


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

    for polygon in polygons:
        # this will be the list of bitstrings of the chosen size
        intersecting_areas: Set[str] = set()

        # perform the BFS
        visited: Dict[str, bool] = {}
        q = deque()
        q.append(
            DiscretizedVoxel.from_coordinate(
                GeodeticCoordinate(
                    longitude=polygon.exterior.coords[0][0],
                    latitude=polygon.exterior.coords[0][1],
                    altitude=0
                )
            ).grow_2d_to_area(
                max_area=polygon.area * f_grow
            )
        )

        while len(q) > 0:
            voxel: DiscretizedVoxel = q.popleft()
            bit_string = voxel.to_bit_string_tuple()[0]

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
                        (1 << (DiscretizedVoxel.Y_BITS - voxel.y_precision))
                    )

                    x_next = (
                        voxel.x_min + dx *
                        (1 << (DiscretizedVoxel.X_BITS - voxel.x_precision))
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
                        y_next = voxel.y_min
                        # if we overflow the x coordinate wraps around
                        x_next = (
                            x_next + math.floor(DiscretizedVoxel.C_X / 2)
                        ) % DiscretizedVoxel.C_X

                    # clear bottom bits of the x coordinate, might be messed up after wrapping around
                    bl = len(bin(x_next)[2:]) - voxel.x_precision
                    if bl > 0:
                        x_next = (x_next >> bl) << bl

                    q.append(
                        DiscretizedVoxel(
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

    assert altitude_min >= DiscretizedVoxel.D and altitude_min <= DiscretizedVoxel.H
    assert altitude_max >= DiscretizedVoxel.D and altitude_max <= DiscretizedVoxel.H
    assert altitude_min <= altitude_max

    discretized_z_min = bin(
        math.floor(
            (altitude_min - DiscretizedVoxel.D) / DiscretizedVoxel.U
        )
    )[2:].rjust(DiscretizedVoxel.Z_BITS, "0")

    discretized_z_max = bin(
        math.floor(
            (altitude_max - DiscretizedVoxel.D) / DiscretizedVoxel.U
        )
    )[2:].rjust(DiscretizedVoxel.Z_BITS, "0")

    bit_string = ""
    for b1, b2 in zip(discretized_z_min, discretized_z_max):
        if b1 != b2:
            break

        bit_string += b1

    # use max, bit string can be longer
    return bit_string


def extruded_polygons_to_bit_string_tuples(
        polygons: List[Polygon],
        altitude_min: float,
        altitude_max: float,
        f_grow: float
) -> List[Tuple[str, str]]:
    """
    Returns the cross product of `polygons_to_2d_bit_strings` and
    `smallest_enclosing_z_bit_string` for the given parameters
    resulting in the set of all bit string tuples where a given
    extruded polygon should be assigned. Always over-approximates,
    i.e. covers the whole extruded polygon.

    Parameters
    ----------
    :param polygons: A list of polygons that should be mapped to voxels
    :param altitude_min: The lower altitude bound for the extruded polygon, [D, altitude_max)
    :param altitude_max: The upper altitude bound for the extruded polygon, (altitude_min, H]
    :param f_grow: The fraction of a polygons area which should be used for the voxel size
    :returns: A set of bit string tuples 
    """

    xy_bit_strings = polygons_to_2d_bit_strings(
        polygons=polygons,
        f_grow=f_grow,
        # always over-approximate
        f_min=0
    )

    z_bit_string = smallest_enclosing_z_bit_string(
        altitude_min=altitude_min,
        altitude_max=altitude_max
    )

    return [
        (xy_bit_string, z_bit_string)
        for xy_bit_string in xy_bit_strings
    ]
