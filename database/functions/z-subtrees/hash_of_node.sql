CREATE OR REPLACE FUNCTION public.hash_of_node(
	input_bit_string_51 bit varying,
	input_bit_string_15 bit varying
)
    RETURNS bytea
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  len_51 integer := LENGTH(input_bit_string_51);
  len_15 integer := LENGTH(input_bit_string_15);

  bit_string_51 bit varying(51);
  bit_string_15 bit varying(15);
  xy_left_child_hash bytea;
  xy_right_child_hash bytea;
  z_left_child_hash bytea;
  z_right_child_hash bytea;
  certificate_hashes bytea[];

  concatenated_certificate_hashes bytea;
BEGIN

-- query the data from the 'nodes' table
SELECT nodes.bit_string_51, nodes.bit_string_15,
nodes.xy_left_child_hash, nodes.xy_right_child_hash,
nodes.z_left_child_hash, nodes.z_right_child_hash,
nodes.certificate_hashes
INTO bit_string_51, bit_string_15,
xy_left_child_hash, xy_right_child_hash,
z_left_child_hash, z_right_child_hash,
certificate_hashes
FROM nodes
WHERE nodes.bit_string_51 = input_bit_string_51
AND   nodes.bit_string_15 = input_bit_string_15;

-- replace the child hashes with SHA256(0) if they are null
SELECT (
  CASE
    WHEN xy_left_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE xy_left_child_hash
  END
) INTO xy_left_child_hash;

SELECT (
  CASE
    WHEN xy_right_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE xy_right_child_hash
  END
) INTO xy_right_child_hash;

SELECT (
  CASE
    WHEN z_left_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE z_left_child_hash
  END
) INTO z_left_child_hash;

SELECT (
  CASE
    WHEN z_right_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE z_right_child_hash
  END
) INTO z_right_child_hash;

-- concatenate the certificate_hashes array
SELECT string_agg(sq.certificate_hash, '')
INTO concatenated_certificate_hashes
FROM (
	SELECT unnest(
    CASE
      WHEN certificate_hashes IS NULL THEN (ARRAY[]::bytea array)
      ELSE certificate_hashes
    END
  ) as certificate_hash
  -- sort before concatenating
  ORDER BY certificate_hash ASC
) sq;

RETURN (
  CASE
    -- if the subtree is empty
    WHEN bit_string_51 IS NULL THEN sha256(bytea '\x00')
    -- the children hash fields of tree leafs must be equal to null
    WHEN (len_51 > 51 OR len_15 > 15) THEN NULL
    -- invalid bit string
    WHEN (len_51 = 0 AND len_15 > 0) THEN NULL
    -- for leaves the hash is H(0 || H(C_0) || H(C_1) | ...)
    WHEN len_51 = 51 and len_15 = 15 THEN sha256(
      bytea '\x00' || concatenated_certificate_hashes
    )
    ELSE (
      CASE
          -- if no certificates, omit last hash
          WHEN cardinality(certificate_hashes) = 0 THEN sha256(
            (bytea '\x01') ||
            xy_left_child_hash ||
            xy_right_child_hash ||
            z_left_child_hash ||
            z_right_child_hash
          )
          -- otherwise add it last
          ELSE sha256(
            (bytea '\x01') ||
            left_child_hash ||
            right_child_hash ||
            z_left_child_hash ||
            z_right_child_hash
            sha256(concatenated_certificate_hashes)
          )
      END
  )
  END
);
END;
$BODY$;
