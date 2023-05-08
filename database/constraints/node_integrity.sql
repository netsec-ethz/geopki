ALTER TABLE IF EXISTS public.nodes
    ADD CONSTRAINT node_integrity CHECK (
      -- ensure the bit string length is bound
      LENGTH(bit_string) <= 66 AND
      -- ensure bit_string_52 and bit_string are consistent
      bit_string_52 =	rpad(
        SUBSTRING(bit_string FROM 1 FOR 52)::text,
        52,
        '0'
      )::bit(52)::bigint AND
      -- ensure the precomuted hashes are correct
      -- if the length is larger than 66, 'hash_of_node' returns NULL
      (
        CASE
          WHEN (
            neighbor_hash IS NULL AND
            hash_of_neighbor(bit_string) IS NULL
          ) THEN TRUE
          WHEN (
            neighbor_hash IS NULL OR
            hash_of_neighbor(bit_string) IS NULL
          ) THEN FALSE
          ELSE (
            neighbor_hash = hash_of_neighbor(bit_string)
          )
        END
      ) AND
      (
        CASE
          WHEN (
            left_child_hash IS NULL AND
            hash_of_node(bit_string || b'0') IS NULL
          ) THEN TRUE
          WHEN (
            left_child_hash IS NULL OR
            hash_of_node(bit_string || b'0') IS NULL
          ) THEN FALSE
          ELSE (
            left_child_hash = hash_of_node(bit_string || b'0')
          )
        END
      ) AND
      (
        CASE
          WHEN (
            right_child_hash IS NULL AND
            hash_of_node(bit_string || b'1') IS NULL
          ) THEN TRUE
          WHEN (
            right_child_hash IS NULL OR
            hash_of_node(bit_string || b'1') IS NULL
          ) THEN FALSE
          ELSE (
            right_child_hash = hash_of_node(bit_string || b'1')
          )
        END
      )
);