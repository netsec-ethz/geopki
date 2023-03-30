import numpy as np
import matplotlib.pyplot as plt
from matplotlib.axes import Axes
from typing import Tuple
import os
from matplotlib.ticker import MaxNLocator

XY_BITS = 2
Z_BITS = 2
B = XY_BITS + XY_BITS + Z_BITS


def get_xy_bitstring(i: int) -> Tuple[str, str]:
    # get bit string of integer i, big endian
    bit_string = str(bin(i))[2:].rjust(B, "0")
    # chop off z bits at the end
    return bit_string[:(-Z_BITS)]


def get_z_bitstring(i: int) -> str:
    # get bit string of integer i, big endian
    bit_string = str(bin(i))[2:].rjust(B, "0")
    # chop off z bits at the end
    return bit_string[-Z_BITS:]


def get_x_coordinate(i: int) -> int:
    # get every other bit of bitstring
    bit_string = get_xy_bitstring(i)[::2]
    # print(str(bin(10))[2:].rjust(B, "0"))
    # print(get_xy_bitstring(10))
    # print(int(bit_string, 2))
    # exit()
    return int(bit_string, 2)


def get_y_coordinate(i: int) -> int:
    # get every other bit of bitstring with an offset of 1
    bit_string = get_xy_bitstring(i)[1::2]

    return int(bit_string, 2)


def get_coords_alt(i: int) -> Tuple[str, str]:
    # get bit string of integer i, big endian
    bit_string = str(bin(i))[2:].rjust(B, "0")
    # chop off z bits at the end
    return int(bit_string[::3], 2), int(bit_string[1::3], 2), int(bit_string[2::3], 2)


def get_z_coordinate(i: int) -> int:
    bit_string = get_z_bitstring(i)

    return int(bit_string, 2)


output_dir = os.path.dirname(os.path.abspath(__file__))

fig = plt.figure(dpi=300)
ax: Axes = fig.add_subplot(projection='3d')

num_points = 2 ** B
xs = np.array([get_x_coordinate(i) for i in range(num_points)])
ys = np.array([get_y_coordinate(i) for i in range(num_points)])
zs = np.array([get_z_coordinate(i) for i in range(num_points)])

us = np.concatenate([xs[1:], xs[-1:]]) - xs
vs = np.concatenate([ys[1:], ys[-1:]]) - ys
ws = np.concatenate([zs[1:], zs[-1:]]) - zs

ax.quiver(
    xs,
    ys,
    zs,
    us,
    vs,
    ws,
    label="order",
    arrow_length_ratio=0.2,
)
ax.scatter(
    xs,
    ys,
    zs,
    label="points"
)


ax.set_xlabel('longitude ($x$)')
ax.set_ylabel('latitude ($y$)')
ax.set_zlabel('evelation ($z$)')
ax.xaxis.set_major_locator(MaxNLocator(integer=True))
ax.yaxis.set_major_locator(MaxNLocator(integer=True))
ax.zaxis.set_major_locator(MaxNLocator(integer=True))

# ax.view_init(elev=20., azim=-35, roll=0)

plt.legend()
plt.savefig(os.path.join(output_dir, "two-bits.png"))


fig = plt.figure(dpi=300)
ax: Axes = fig.add_subplot(projection='3d')

xs = np.array([get_coords_alt(i)[0] for i in range(num_points)])
ys = np.array([get_coords_alt(i)[1] for i in range(num_points)])
zs = np.array([get_coords_alt(i)[2] for i in range(num_points)])

us = np.concatenate([xs[1:], xs[-1:]]) - xs
vs = np.concatenate([ys[1:], ys[-1:]]) - ys
ws = np.concatenate([zs[1:], zs[-1:]]) - zs

ax.quiver(
    xs,
    ys,
    zs,
    us,
    vs,
    ws,
    label="order",
    arrow_length_ratio=0.2,
)
ax.scatter(
    xs,
    ys,
    zs,
    label="points",
)

ax.set_xlabel('longitude ($x$)')
ax.set_ylabel('latitude ($y$)')
ax.set_zlabel('evelation ($z$)')
ax.xaxis.set_major_locator(MaxNLocator(integer=True))
ax.yaxis.set_major_locator(MaxNLocator(integer=True))
ax.zaxis.set_major_locator(MaxNLocator(integer=True))

plt.legend()
plt.savefig(os.path.join(output_dir, "two-bits-z.png"))
