CREATE OR REPLACE FUNCTION public.compute_hashes()
    RETURNS void
    LANGUAGE 'plpgsql'
    COST 100
    VOLATILE PARALLEL UNSAFE
AS $BODY$
BEGIN
-- iterate over the full 2D tree depth in reverse
FOR xy_bit_string_depth IN REVERSE 51..0 LOOP
  -- iterate over the full Z subtree in reverse
  FOR z_bit_string_depth IN REVERSE 15..0 LOOP
    IF z_bit_string_depth = 0 THEN
      -- node in the "normal" 2D tree, not a z subtree
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
          FULL OUTER JOIN nodes as xy_left_child
            ON (
              xy_left_child.bit_string_51 = nodes.bit_string_51 || b'0' AND
              xy_left_child.bit_string_15 = nodes.bit_string_15  -- b''
            )
          -- right child in the 2D tree
          FULL OUTER JOIN nodes as xy_right_child
            ON (
              xy_right_child.bit_string_51 = nodes.bit_string_51 || b'1' AND
              xy_right_child.bit_string_15 = nodes.bit_string_15  -- b''
            )
          -- left child in the z subtree
          FULL OUTER JOIN nodes as z_left_child
            ON (
              z_left_child.bit_string_51 = nodes.bit_string_51 AND
              z_left_child.bit_string_15 = nodes.bit_string_15 || b'0'
            )
          -- right child in the z subtree
          FULL OUTER JOIN nodes as z_right_child
            ON (
              z_right_child.bit_string_51 = nodes.bit_string_51 AND
              z_right_child.bit_string_15 = nodes.bit_string_15 || b'1'
            )
        -- conditions on the original table
        WHERE LENGTH(nodes.bit_string_51) = xy_bit_string_depth
        AND   LENGTH(nodes.bit_string_15) = z_bit_string_depth
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
      -- z_bit_string_depth \in (1, 15)
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
          FULL OUTER JOIN nodes as z_left_child
            ON (
              z_left_child.bit_string_51 = nodes.bit_string_51 AND
              z_left_child.bit_string_15 = nodes.bit_string_15 || b'0'
            )
          -- right child in the z subtree
          FULL OUTER JOIN nodes as z_right_child
            ON (
              z_right_child.bit_string_51 = nodes.bit_string_51 AND
              z_right_child.bit_string_15 = nodes.bit_string_15 || b'1'
            )
        -- conditions on the original table
        WHERE LENGTH(nodes.bit_string_51) = xy_bit_string_depth
        AND   LENGTH(nodes.bit_string_15) = z_bit_string_depth
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
  END LOOP;
END LOOP;

END;
$BODY$;
