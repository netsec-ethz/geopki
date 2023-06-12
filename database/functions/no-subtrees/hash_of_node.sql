-- FUNCTION: public.hash_of_node(bit varying)

CREATE OR REPLACE FUNCTION public.hash_of_node(
	input_bit_string bit varying
)
    RETURNS bytea
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  len integer := LENGTH(input_bit_string);

	bit_string bit varying;
	left_child_hash bytea;
  right_child_hash bytea;
  certificate_hashes bytea[];
  concatenated_certificate_hashes bytea;
BEGIN

-- query the data from the 'nodes' table
SELECT nodes.bit_string, nodes.left_child_hash, nodes.right_child_hash, nodes.certificate_hashes
INTO bit_string, left_child_hash, right_child_hash, certificate_hashes
FROM nodes
WHERE nodes.bit_string = input_bit_string;

-- replace the child hashes with SHA256(0) if they are null
-- TODO: check if has to be changed to recursively compute the hash
SELECT (
  CASE
    WHEN left_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE left_child_hash
  END
) INTO left_child_hash;

SELECT (
  CASE
    WHEN right_child_hash IS NULL THEN sha256(bytea '\x00')
    ELSE right_child_hash
  END
) INTO right_child_hash;

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
    WHEN bit_string IS NULL THEN sha256(bytea '\x00')
    -- the children hash fields of tree leafs must be equal to null
    WHEN len > 66 THEN NULL
    -- for leaves the hash is H(0 || H(C_0) || H(C_1) | ...)
    WHEN len = 66 THEN sha256(
      bytea '\x00' || concatenated_certificate_hashes
    )
    ELSE (
      CASE
          -- if no certificates, omit last hash
          WHEN cardinality(certificate_hashes) = 0 THEN sha256(
            (bytea '\x01') ||
            left_child_hash ||
            right_child_hash
          )
          -- otherwise add it last
          ELSE sha256(
            (bytea '\x01') ||
            left_child_hash ||
            right_child_hash ||
            sha256(concatenated_certificate_hashes)
          )
      END
  )
  END
);
END;
$BODY$;

ALTER FUNCTION public.hash_of_node(bit varying)
    OWNER TO postgis_user;
