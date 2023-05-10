ALTER TABLE IF EXISTS public.nodes
    ADD CONSTRAINT node_integrity CHECK (
      -- ensure the bit string length is bound
      LENGTH(bit_string_51) > 0 AND
      LENGTH(bit_string_51) <= 51 AND
      LENGTH(bit_string_15) <= 15 AND
      -- ensure bit_string_51 and bit_string are consistent
      bit_string_51 =	rpad(
        SUBSTRING(bit_string FROM 1 FOR 51)::text,
        51,
        '0'
      )::bit(51)::bigint AND
      -- ensure the precomuted hashes are correct
      -- if the length is larger than 66, 'hash_of_node' returns NULL
      (
        CASE
          WHEN (
            neighbor_hash IS NULL AND
            hash_of_neighbor(
              bit_string_51,
              bit_string_15
            ) IS NULL
          ) THEN TRUE
          WHEN (
            neighbor_hash IS NULL OR
            hash_of_neighbor(
              bit_string_51,
              bit_string_15
            ) IS NULL
          ) THEN FALSE
          ELSE (
            neighbor_hash = hash_of_neighbor(
              bit_string_51,
              bit_string_15
            )
          )
        END
      ) AND
      (
        CASE
          WHEN (
            xy_left_child_hash IS NULL AND
            hash_of_node(bit_string_51 || b'0', bit_string_15) IS NULL
          ) THEN TRUE
          WHEN (
            xy_left_child_hash IS NULL OR
            hash_of_node(bit_string_51 || b'0', bit_string_15) IS NULL
          ) THEN FALSE
          ELSE (
            xy_left_child_hash = hash_of_node(bit_string || b'0', bit_string_15)
          )
        END
      ) AND
      (
        CASE
          WHEN (
            xy_right_child_hash IS NULL AND
            hash_of_node(bit_string || b'1', bit_string_15) IS NULL
          ) THEN TRUE
          WHEN (
            xy_right_child_hash IS NULL OR
            hash_of_node(bit_string || b'1', bit_string_15) IS NULL
          ) THEN FALSE
          ELSE (
            xy_right_child_hash = hash_of_node(bit_string || b'1', bit_string_15)
          )
        END
      ) AND
      (
        CASE
          WHEN (
            z_left_child_hash IS NULL AND
            hash_of_node(bit_string_51, bit_string_15 || b'0') IS NULL
          ) THEN TRUE
          WHEN (
            z_left_child_hash IS NULL OR
            hash_of_node(bit_string_51, bit_string_15 || b'0') IS NULL
          ) THEN FALSE
          ELSE (
            z_left_child_hash = hash_of_node(bit_string, bit_string_15 || b'0')
          )
        END
      ) AND
      (
        CASE
          WHEN (
            z_right_child_hash IS NULL AND
            hash_of_node(bit_string, bit_string_15 || b'1') IS NULL
          ) THEN TRUE
          WHEN (
            z_right_child_hash IS NULL OR
            hash_of_node(bit_string, bit_string_15 || b'1') IS NULL
          ) THEN FALSE
          ELSE (
            z_right_child_hash = hash_of_node(bit_string, bit_string_15 || b'1')
          )
        END
      )
);