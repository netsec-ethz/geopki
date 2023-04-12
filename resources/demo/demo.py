from __future__ import annotations
from typing import List, Tuple, Dict, Union
import math
import sys
from geopy import distance

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
        assert altitude >= - 10000 and \
            altitude <= ZOrderBinaryString.C_Z

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


class ZOrderBinaryString:
    X_BITS = 26
    Y_BITS = 25
    Z_BITS = 15
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

        assert x_min >= 0 and x_min <= ZOrderBinaryString.C_X
        assert y_min >= 0 and y_min <= ZOrderBinaryString.C_Y
        assert z_min >= 0 and z_min <= ZOrderBinaryString.C_Z

        assert x_precision >= 0 and x_precision <= ZOrderBinaryString.X_BITS
        assert y_precision >= 0 and y_precision <= ZOrderBinaryString.Y_BITS
        assert z_precision >= 0 and z_precision <= ZOrderBinaryString.Z_BITS

        # ensure that the lowest ZOrderBinaryString.X_BITS - x_precision bits are cleared
        assert all(bit == '0' for bit in bin(x_min)[2:][x_precision:])
        assert all(bit == '0' for bit in bin(y_min)[2:][y_precision:])
        assert all(bit == '0' for bit in bin(z_min)[2:][z_precision:])

        # more efficient variant:
        # assert (x_min % (1 << (ZOrderBinaryString.X_BITS - x_precision))) == 0

        self.x_min = x_min
        self.y_min = y_min
        self.z_min = z_min

        self.x_max = x_min + \
            (1 << (ZOrderBinaryString.X_BITS - x_precision))
        self.y_max = y_min + \
            (1 << (ZOrderBinaryString.Y_BITS - y_precision))
        self.z_max = z_min + \
            (1 << (ZOrderBinaryString.Z_BITS - z_precision))

    def __str__(self) -> str:
        return f"""
X: [{self.x_min}, {self.x_max})
Y: [{self.y_min}, {self.y_max})
Z: [{self.z_min}, {self.z_max})
        """.strip()

    @staticmethod
    def discretized_values_to_binary_string(x: int, y: int, z: int) -> str:
        assert x <= ZOrderBinaryString.C_X
        assert y <= ZOrderBinaryString.C_Y
        assert z <= ZOrderBinaryString.C_Z

        x_bits = bin(x)[2:].rjust(ZOrderBinaryString.X_BITS, '0')
        y_bits = bin(y)[2:].rjust(ZOrderBinaryString.Y_BITS, '0')

        interleaved_xy_bits = "".join(
            i + j for i, j in zip(
                x_bits,
                y_bits
            )
        ) + x_bits[-1:]  # X_BITS = Y_BITS + 1

        return interleaved_xy_bits + bin(z)[2:].rjust(ZOrderBinaryString.Z_BITS, '0')

    def to_binary_string(self) -> str:
        return ZOrderBinaryString.discretized_values_to_binary_string(
            self.x_min,
            self.y_min,
            self.z_min
        )

    @staticmethod
    def msb_binary_string_to_int(bitstring: str, max_bits: int) -> int:
        assert len(bitstring) <= max_bits

        a = 0
        for i, bit in enumerate(bitstring):
            power = max_bits - 1 - i
            a += (1 << power) * (bit == '1')

        return a

    @staticmethod
    def undiscretize(x: int, y: int, z: int) -> GeodeticCoordinate:
        return GeodeticCoordinate(
            longitude=(x * 360 / ZOrderBinaryString.C_X - 180),
            latitude=(y * 180 / ZOrderBinaryString.C_Y - 90),
            altitude=(z + ZOrderBinaryString.D)
        )

    def to_coordinate(self) -> GeodeticCoordinate:
        return ZOrderBinaryString.undiscretize(
            self.x_min,
            self.y_min,
            self.z_min
        )

    def to_voxel_bounds(self) -> Tuple[GeodeticCoordinate, GeodeticCoordinate]:
        return (
            ZOrderBinaryString.undiscretize(
                self.x_min,
                self.y_min,
                self.z_min
            ),
            ZOrderBinaryString.undiscretize(
                self.x_max,
                self.y_max,
                self.z_max
            )
        )

    def to_voxel(self) -> List[GeodeticCoordinate]:
        return [
            ZOrderBinaryString.undiscretize(
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

    def eucledian_distance_voxel_voxel(self, other: ZOrderBinaryString) -> float:
        v1 = self.to_voxel()
        v2 = other.to_voxel()

        return min(
            p.eucledian_distance(q)
            for p in v1
            for q in v2
        )

    @staticmethod
    def from_binary_string(
        binary_string: int
    ) -> ZOrderBinaryString:
        bitstring = bin(binary_string)[2:]
        assert len(bitstring) <= ZOrderBinaryString.MAX_BITSTRING_LENGTH

        # B_{x} + B_{y}
        xyBits = ZOrderBinaryString.X_BITS + ZOrderBinaryString.Y_BITS

        # get every other coordinate starting at 0 up to 2 * B_{x,y}
        x_coordinate = bitstring[0:xyBits:2]
        # get every other coordinate starting at 1 up to 2 * B_{x,y}
        y_coordinate = bitstring[1:xyBits:2]
        # get all bits after 2 * B_{x,y}
        z_coordinate = bitstring[xyBits:]

        x_min = ZOrderBinaryString.msb_binary_string_to_int(
            x_coordinate,
            ZOrderBinaryString.X_BITS
        )

        y_min = ZOrderBinaryString.msb_binary_string_to_int(
            y_coordinate,
            ZOrderBinaryString.Y_BITS
        )

        z_min = ZOrderBinaryString.msb_binary_string_to_int(
            z_coordinate,
            ZOrderBinaryString.Z_BITS
        )

        return ZOrderBinaryString(
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
    ) -> ZOrderBinaryString:
        x = math.floor(
            (
                (coordinate.longitude + 180) / 360
            ) * ZOrderBinaryString.C_X
        )

        y = math.floor(
            (
                (coordinate.latitude + 90) / 180
            ) * ZOrderBinaryString.C_Y
        )

        z = math.floor(coordinate.altitude - ZOrderBinaryString.D)

        return ZOrderBinaryString(
            x_min=x,
            x_precision=ZOrderBinaryString.X_BITS,
            y_min=y,
            y_precision=ZOrderBinaryString.Y_BITS,
            z_min=z,
            z_precision=ZOrderBinaryString.Z_BITS
        )


def sphere_to_binary_strings(
        center: GeodeticCoordinate,
        radius_m: float
) -> List[ZOrderBinaryString]:
    # add maximum diagonal of a voxel to the radius
    # this over-approximates the sphere but ensures we only have to
    # check the voxel's vertices to be within the sphere's radius
    MAX_VOXEL_DIAGONAL = math.ceil(math.sqrt(3))

    radius_m += MAX_VOXEL_DIAGONAL

    cartesian_center = center.to_cartesian()
    memo: Dict[Tuple[int, int, int], bool] = {}

    center_voxel = ZOrderBinaryString.from_coordinate(
        coordinate=center
    )
    binary_strings: List[ZOrderBinaryString] = [center_voxel]
    memo[(center_voxel.x_min, center_voxel.y_min, center_voxel.z_min)] = True
    i = 0

    while i < len(binary_strings):
        s = binary_strings[i]

        # if (x, y, z) in memo:
        # this voxel was already visited
        # return

        # print(p.eucledian_distance_voxel_point(cartesian_center))
        # print(p.to_coordinate())
        # if it is within the radius, append to list and visit all neighbors

        for dx in [-1, 0, 1]:
            for dy in [-1, 0, 1]:
                for dz in [-1, 0, 1]:

                    x = s.x_min + dx
                    y = s.y_min + dy
                    z = s.z_min + dz

                    # if the poles are crossed, the longitude flips by 180 degrees
                    if y < 0:
                        y = -y
                        # it is fine if we overflow, the x coordinate wraps around
                        x += math.floor(ZOrderBinaryString.C_X / 2)
                    elif y >= ZOrderBinaryString.C_Y:
                        y = ZOrderBinaryString.C_Y - \
                            (y - ZOrderBinaryString.C_Y)
                        # it is fine if we overflow, the x coordinate wraps around
                        x += math.floor(ZOrderBinaryString.C_X / 2)

                    # the longitudal axis wraps around
                    x = x % ZOrderBinaryString.C_X

                    # abort if out of bounds
                    if z < 0 or z > ZOrderBinaryString.C_Z:
                        continue

                    if (x, y, z) in memo:
                        # this voxel was already visited
                        continue

                    # if not, mark as visited
                    memo[(x, y, z)] = True

                    # check if the voxel is within the sphere's radius
                    p = ZOrderBinaryString(
                        x_min=x,
                        x_precision=ZOrderBinaryString.X_BITS,
                        y_min=y,
                        y_precision=ZOrderBinaryString.Y_BITS,
                        z_min=z,
                        z_precision=ZOrderBinaryString.Z_BITS
                    )
                    if p.eucledian_distance_voxel_point(cartesian_center) > radius_m:
                        # if not, skip this one
                        continue

                    # if it is, append to the list
                    binary_strings.append(p)

        # after iterating over an element increase the index i
        i += 1

    # distance.distance(
    #     kilometers=scan_unit_size_km
    # ).destination(
    #     (s, w),
    #     bearing=0
    # ).latitude
    return binary_strings


# random example:
# 66 bits: 100101100010100111010011111100110110000110111110011011010101000001
# 57 bits: 100101100010100111010011111100110110000110111110011011010
# print(ZOrderBinaryString.msb_binary_string_to_int("0111", 5))

# example from pdf:
x_bits = "10000110000101000010101010".rjust(ZOrderBinaryString.X_BITS, '0')
y_bits = "1100001101100001001110101".rjust(ZOrderBinaryString.Y_BITS, '0')
z_bits = "10011100011010".rjust(ZOrderBinaryString.Z_BITS, '0')
binary_string = "".join(
    i + j for i, j in zip(
                x_bits,
                y_bits
    )
) + x_bits[-1:] + z_bits

b = ZOrderBinaryString.from_binary_string(
    int("10010110001010", 2)
)

# print(
#     b
# )
# print(b.to_binary_string())
# print(b.to_coordinate())

# u = ZOrderBinaryString.from_binary_string(
#     int("11010000001011010001011000100001000011011100110011", 2)
# )
# v = ZOrderBinaryString.from_binary_string(
#     int("110100000010110100010110001000010000110111001100111", 2)
# )

# print(
#     ZOrderBinaryString.from_coordinate(
#         GeodeticCoordinate(
#             latitude=89,
#             longitude=15.547839,
#             altitude=0
#         )
#     )
# )
# print(
#     ZOrderBinaryString.from_coordinate(
#         GeodeticCoordinate(
#             latitude=89,
#             longitude=(((15.547839 + 180) + 180) % 360) - 180,
#             altitude=0
#         )
#     )
# )

u = ZOrderBinaryString.from_coordinate(
    GeodeticCoordinate(
        latitude=41.466138,
        longitude=15.547839,
        altitude=0
    )
)

v = ZOrderBinaryString.from_coordinate(
    GeodeticCoordinate(
        latitude=41.467216,
        longitude=15.547025,
        altitude=0
    )
)

# print(u)
# print(v)
# print(u.to_coordinate())
# print(distance.distance((41.466138, 15.547839), (41.467216, 15.547025)).m)
# print(u.to_coordinate().distance_on_geoid(
#     v.to_coordinate()))
# print(u.to_coordinate().eucledian_distance(
#     v.to_coordinate()))

# print(u.eucledian_distance_voxel_voxel(v))

l = sphere_to_binary_strings(
    GeodeticCoordinate(
        latitude=85.467216,
        longitude=15.547025,
        altitude=0
    ),
    9
)

print(len(l))

string_list = set(x.to_binary_string() for x in l)
print(len(string_list))
