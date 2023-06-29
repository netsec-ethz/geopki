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
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51 || b'0'
                AND children.bit_string_15 = nodes.bit_string_15
              ) THEN
                smt_hash(
                  children.bit_string_51,
                  children.bit_string_15,
                  children.xy_left_child_hash,
                  children.xy_right_child_hash,
                  children.z_left_child_hash,
                  children.z_right_child_hash,
                  children.certificate_hashes
                )
              ELSE NULL
            END,
          NULL
          ) as new_xy_left_child_hash,
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51 || b'1'
                AND children.bit_string_15 = nodes.bit_string_15
              ) THEN
              smt_hash(
                children.bit_string_51,
                children.bit_string_15,
                children.xy_left_child_hash,
                children.xy_right_child_hash,
                children.z_left_child_hash,
                children.z_right_child_hash,
                children.certificate_hashes
              )
              ELSE NULL
            END,
          NULL
          ) as new_xy_right_child_hash,
          -- compute the children hashes of the z subtree using the joined data
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51
                AND children.bit_string_15 = nodes.bit_string_15 || b'0'
              ) THEN
                smt_hash(
                  children.bit_string_51,
                  children.bit_string_15,
                  children.xy_left_child_hash,
                  children.xy_right_child_hash,
                  children.z_left_child_hash,
                  children.z_right_child_hash,
                  children.certificate_hashes
                )
              ELSE NULL
            END,
          NULL
          ) as new_z_left_child_hash,
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51
                AND children.bit_string_15 = nodes.bit_string_15 || b'1'
              ) THEN
              smt_hash(
                children.bit_string_51,
                children.bit_string_15,
                children.xy_left_child_hash,
                children.xy_right_child_hash,
                children.z_left_child_hash,
                children.z_right_child_hash,
                children.certificate_hashes
              )
              ELSE NULL
            END,
          NULL
          ) as new_z_right_child_hash
        
        FROM
          nodes,
          nodes as children
          -- conditions on the original table
          WHERE LENGTH(nodes.bit_string_51) = xy_bit_string_depth
          AND   LENGTH(nodes.bit_string_15) = z_bit_string_depth
        -- join conditions
        AND (
          (
            LENGTH(children.bit_string_51) = xy_bit_string_depth
            AND nodes.bit_string_51 = children.bit_string_51
            AND LENGTH(children.bit_string_15) = (z_bit_string_depth + 1)
            AND (
              children.bit_string_15 = nodes.bit_string_15 || b'0'
              OR children.bit_string_15 = nodes.bit_string_15 || b'1'
            )
          )
          OR (
            LENGTH(children.bit_string_51) = (xy_bit_string_depth + 1)
            AND (
              children.bit_string_51 = nodes.bit_string_51 || b'0'
              OR children.bit_string_51 = nodes.bit_string_51 || b'1'
            )
            AND LENGTH(children.bit_string_15) = z_bit_string_depth
            AND nodes.bit_string_15 = children.bit_string_15
          )
        )
        GROUP BY nodes.bit_string_51, nodes.bit_string_15
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
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51
                AND children.bit_string_15 = nodes.bit_string_15 || b'0'
              ) THEN
                smt_hash(
                  children.bit_string_51,
                  children.bit_string_15,
                  children.xy_left_child_hash,
                  children.xy_right_child_hash,
                  children.z_left_child_hash,
                  children.z_right_child_hash,
                  children.certificate_hashes
                )
              ELSE NULL
            END,
          NULL
          ) as new_z_left_child_hash,
          string_agg(
            CASE
              WHEN (
                children.bit_string_51 = nodes.bit_string_51
                AND children.bit_string_15 = nodes.bit_string_15 || b'1'
              ) THEN
              smt_hash(
                children.bit_string_51,
                children.bit_string_15,
                children.xy_left_child_hash,
                children.xy_right_child_hash,
                children.z_left_child_hash,
                children.z_right_child_hash,
                children.certificate_hashes
              )
              ELSE NULL
            END,
          NULL
          ) as new_z_right_child_hash
        
        FROM
          nodes,
          nodes as children
          -- conditions on the original table
          WHERE LENGTH(nodes.bit_string_51) = xy_bit_string_depth
          AND   LENGTH(nodes.bit_string_15) = z_bit_string_depth
        -- join conditions
        AND (
          (
            LENGTH(children.bit_string_51) = xy_bit_string_depth
            AND nodes.bit_string_51 = children.bit_string_51
            AND LENGTH(children.bit_string_15) = (z_bit_string_depth + 1)
            AND (
              children.bit_string_15 = nodes.bit_string_15 || b'0'
              OR children.bit_string_15 = nodes.bit_string_15 || b'1'
            )
          )
          OR (
            LENGTH(children.bit_string_51) = (xy_bit_string_depth + 1)
            AND (
              children.bit_string_51 = nodes.bit_string_51 || b'0'
              OR children.bit_string_51 = nodes.bit_string_51 || b'1'
            )
            AND LENGTH(children.bit_string_15) = z_bit_string_depth
            AND nodes.bit_string_15 = children.bit_string_15
          )
        )
        GROUP BY nodes.bit_string_51, nodes.bit_string_15
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
