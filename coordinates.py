from __future__ import annotations
from typing import List, Tuple, Union, Dict
import math
from collections import deque
from geopy import distance
from shapely import box, Point, Polygon

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


def grow_area(initial_area: ZOrderBitString, area: float, f: float):
    # while the area of the object is larger than a fraction of the grid's area
    # decrease the grid's size
    while (
        initial_area.x_precision > 1 and
        ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        ).to_shapely_area().area * f < area
    ):

        # grow area by removing one bit
        initial_area = ZOrderBitString.from_bit_string(
            initial_area.to_bit_string()[:-1]
        )

    return initial_area


def sphere_to_coarse_2d_binary_strings(
        center: GeodeticCoordinate,
        radius_m: float,
        f_grow: float,
        f_min: float
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

    # this will be the list of bitstrings of the chosen size
    intersecting_areas: List[str] = []
    results: List[str] = []

    visited: Dict[str, bool] = {}
    q = deque()
    q.append(
        grow_area(
            initial_area=ZOrderBitString.from_coordinate(center),
            area=circle.area,
            f=f_grow
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
            asa.intersection(circle).area > f_min * asa.area
        ):

            continue

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

    return results
