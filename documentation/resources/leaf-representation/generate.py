import numpy as np
import matplotlib.pyplot as plt
from matplotlib.axes import Axes
from typing import Tuple
import os
from matplotlib.ticker import MaxNLocator

output_dir = os.path.dirname(os.path.abspath(__file__))

fig = plt.figure(dpi=300)
ax: Axes = fig.add_subplot(projection='3d', computed_zorder=False)

# prepare some coordinates
x, y, z = np.indices((3, 3, 3))

# draw cuboids in the top left and bottom right corners, and a link between
# them
cube1 = (x >= 0) & (x < 1) & (y >= 0) & (y < 1) & (z >= 0) & (z < 1)
cube2 = (x >= 1) & (x < 2) & (y >= 1) & (y < 2) & (z >= 0) & (z < 1)
cube3 = (x >= 2) & (x < 3) & (y >= 2) & (y < 3) & (z >= 0) & (z < 1)
cube4 = (x >= 1) & (x < 2) & (y >= 1) & (y < 2) & (z >= 1) & (z < 2)

# combine the objects into a single boolean array
voxelarray = cube1 | cube2 | cube3 | cube4

# set the colors of each object
colors = np.empty(voxelarray.shape, dtype=object)
colors[cube1] = 'blue'
colors[cube2] = 'orange'
colors[cube3] = 'green'
colors[cube4] = 'red'

ax.voxels(
    voxelarray,
    facecolors=colors,
    edgecolor='k',
    alpha=0.5,
    zorder=4.4
)

ax.scatter(
    [0, 1, 2, 1],
    [0, 1, 2, 1],
    [0, 0, 0, 1],
    c=[
        "blue",
        "orange",
        "green",
        "red"
    ],
    edgecolor='k',
    s=50,
    label="points",
    zorder=4.5,
    alpha=1
)


ax.set_xlabel('longitude ($x$)')
ax.set_ylabel('latitude ($y$)')
ax.set_zlabel('evelation ($z$)')
# ax.set_xlim(0, 4)
# ax.set_ylim(0, 4)
# ax.set_zlim(0, 4)
ax.xaxis.set_major_locator(MaxNLocator(integer=True))
ax.yaxis.set_major_locator(MaxNLocator(integer=True))
ax.zaxis.set_major_locator(MaxNLocator(integer=True))

# ax.view_init(elev=20., azim=-35, roll=0)

# plt.legend()
plt.savefig(os.path.join(output_dir, "geometric-representation.png"))
