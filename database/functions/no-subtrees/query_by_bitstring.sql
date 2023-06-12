-- FUNCTION: public.query_by_bitstring(bit varying(66), smallint, integer)

CREATE OR REPLACE FUNCTION public.query_by_bitstring(
	query_bit_string bit varying(66),
	query_z smallint,
	query_radius integer
)
    RETURNS TABLE (
		bit_string bit varying(66),
		certificate_hashes bytea[],
		left_child_hash bytea,
		right_child_hash bytea
	)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  query_bit_string_52 bit varying(52) := SUBSTRING(query_bit_string FROM 1 FOR 52);
BEGIN
RETURN QUERY (
  SELECT
    nodes.bit_string, nodes.certificate_hashes,
    nodes.left_child_hash, nodes.right_child_hash
  FROM nodes
WHERE
  bit_string_52 >= (
    rpad(
      query_bit_string_52::text,
      52,
      '0'
    )::bit(52)::bigint
  ) AND
  bit_string_52 <= (
    rpad(
      query_bit_string_52::text,
      52,
      '1'
    )::bit(52)::bigint
  ) AND
  min_altitude_of_bit_string(nodes.bit_string) <= (query_z + query_radius) AND
  max_altitude_of_bit_string(nodes.bit_string) >= (query_z - query_radius)
);
END;
$BODY$;