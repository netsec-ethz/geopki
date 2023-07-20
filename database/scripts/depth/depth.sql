SELECT LENGTH(bit_string_51) as smt_depth_xy, LENGTH(bit_string_15) as smt_depth_z
FROM nodes

-- is sparse / child
WHERE xy_left_child_hash IS NULL
AND   xy_right_child_hash IS NULL
AND    z_left_child_hash IS NULL
AND    z_right_child_hash IS NULL