CREATE OR REPLACE FUNCTION update_children_hashes(
  input_bit_string_51 bit varying,
	input_bit_string_15 bit varying
)
    RETURNS void
    LANGUAGE 'plpgsql'
    COST 100
    VOLATILE PARALLEL UNSAFE
AS $BODY$
BEGIN
IF LENGTH(input_bit_string_15) = 0 THEN
  WITH node_hashes AS (
    SELECT
      nodes.bit_string_51 as bit_string_51,
      nodes.bit_string_15 as bit_string_15,
      -- compute the children hashes of in the 2D tree using the joined data
      smt_hash(
        xy_left_child.bit_string_51,
        xy_left_child.bit_string_15,
        xy_left_child.xy_left_child_hash,
        xy_left_child.xy_right_child_hash,
        xy_left_child.z_left_child_hash,
        xy_left_child.z_right_child_hash,
        xy_left_child.certificate_hashes
      ) as new_xy_left_child_hash,
      smt_hash(
        xy_right_child.bit_string_51,
        xy_right_child.bit_string_15,
        xy_right_child.xy_left_child_hash,
        xy_right_child.xy_right_child_hash,
        xy_right_child.z_left_child_hash,
        xy_right_child.z_right_child_hash,
        xy_right_child.certificate_hashes
      ) as new_xy_right_child_hash,
      -- compute the children hashes of the z subtree using the joined data
      smt_hash(
        z_left_child.bit_string_51,
        z_left_child.bit_string_15,
        z_left_child.xy_left_child_hash,
        z_left_child.xy_right_child_hash,
        z_left_child.z_left_child_hash,
        z_left_child.z_right_child_hash,
        z_left_child.certificate_hashes
      ) as new_z_left_child_hash,
      smt_hash(
        z_right_child.bit_string_51,
        z_right_child.bit_string_15,
        z_right_child.xy_left_child_hash,
        z_right_child.xy_right_child_hash,
        z_right_child.z_left_child_hash,
        z_right_child.z_right_child_hash,
        z_right_child.certificate_hashes
      ) as new_z_right_child_hash
    FROM
      nodes
      -- left child in the 2D tree
      LEFT JOIN nodes as xy_left_child
        ON (
          xy_left_child.bit_string_51 = nodes.bit_string_51 || b'0' AND
          xy_left_child.bit_string_15 = nodes.bit_string_15  -- b''
        )
      -- right child in the 2D tree
      LEFT JOIN nodes as xy_right_child
        ON (
          xy_right_child.bit_string_51 = nodes.bit_string_51 || b'1' AND
          xy_right_child.bit_string_15 = nodes.bit_string_15  -- b''
        )
      -- left child in the z subtree
      LEFT JOIN nodes as z_left_child
        ON (
          z_left_child.bit_string_51 = nodes.bit_string_51 AND
          z_left_child.bit_string_15 = nodes.bit_string_15 || b'0'
        )
      -- right child in the z subtree
      LEFT JOIN nodes as z_right_child
        ON (
          z_right_child.bit_string_51 = nodes.bit_string_51 AND
          z_right_child.bit_string_15 = nodes.bit_string_15 || b'1'
        )
    -- conditions on the original table
    WHERE nodes.bit_string_51 = input_bit_string_51
    AND   nodes.bit_string_15 = input_bit_string_15 -- b''
  )
  UPDATE nodes
  SET
    xy_left_child_hash = new_xy_left_child_hash,
    xy_right_child_hash = new_xy_right_child_hash,
    z_left_child_hash = new_z_left_child_hash,
    z_right_child_hash = new_z_right_child_hash
  
  FROM node_hashes
  WHERE nodes.bit_string_51 = node_hashes.bit_string_51
  AND   nodes.bit_string_15 = node_hashes.bit_string_15;
ELSE
  WITH node_hashes AS (
    SELECT
      nodes.bit_string_51 as bit_string_51,
      nodes.bit_string_15 as bit_string_15,
      -- in the z subtrees all xy_left_child_hash and xy_right_child_hash
      -- are null
      NULL::bytea as new_xy_left_child_hash,
      NULL::bytea as new_xy_right_child_hash,
      -- compute the children hashes of the z subtree using the joined data
      smt_hash(
        z_left_child.bit_string_51,
        z_left_child.bit_string_15,
        z_left_child.xy_left_child_hash,
        z_left_child.xy_right_child_hash,
        z_left_child.z_left_child_hash,
        z_left_child.z_right_child_hash,
        z_left_child.certificate_hashes
      ) as new_z_left_child_hash,
      smt_hash(
        z_right_child.bit_string_51,
        z_right_child.bit_string_15,
        z_right_child.xy_left_child_hash,
        z_right_child.xy_right_child_hash,
        z_right_child.z_left_child_hash,
        z_right_child.z_right_child_hash,
        z_right_child.certificate_hashes
      ) as new_z_right_child_hash
    FROM
      nodes
      -- left child in the z subtree
      LEFT JOIN nodes as z_left_child
        ON (
          z_left_child.bit_string_51 = nodes.bit_string_51 AND
          z_left_child.bit_string_15 = nodes.bit_string_15 || b'0'
        )
      -- right child in the z subtree
      LEFT JOIN nodes as z_right_child
        ON (
          z_right_child.bit_string_51 = nodes.bit_string_51 AND
          z_right_child.bit_string_15 = nodes.bit_string_15 || b'1'
        )
    -- conditions on the original table
    WHERE nodes.bit_string_51 = input_bit_string_51
    AND   nodes.bit_string_15 = input_bit_string_15
  )
  UPDATE nodes
  SET
    xy_left_child_hash = new_xy_left_child_hash,
    xy_right_child_hash = new_xy_right_child_hash,
    z_left_child_hash = new_z_left_child_hash,
    z_right_child_hash = new_z_right_child_hash

  FROM node_hashes
  WHERE nodes.bit_string_51 = node_hashes.bit_string_51
  AND   nodes.bit_string_15 = node_hashes.bit_string_15;
END IF;
END;
$BODY$;
