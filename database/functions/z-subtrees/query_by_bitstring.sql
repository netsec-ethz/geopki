CREATE OR REPLACE FUNCTION public.query_by_bitstring(
	query_bit_string bit varying(66),
	query_z smallint,
	query_radius integer
)
    RETURNS TABLE (
		bit_string_51 bit varying(51),
		bit_string_15 bit varying(15),
		certificate_hashes bytea[],
		neighbor_hash bytea,
		left_child_hash bytea,
		right_child_hash bytea,
		altitude_child_hash bytea
	)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  query_bit_string_51 bit varying(51) := SUBSTRING(query_bit_string FROM 1 FOR 51);
BEGIN
RETURN QUERY (
  SELECT
    nodes.bit_string_51, nodes.bit_string_15, nodes.certificate_hashes,
    nodes.neighbor_hash, nodes.left_child_hash, nodes.right_child_hash, nodes.altitude_child_hash
  FROM nodes
WHERE
  bit_string_51_int >= (
    rpad(
      query_bit_string_51::text,
      51,
      '0'
    )::bit(51)::bigint
  ) AND
  bit_string_51_int <= (
    rpad(
      query_bit_string_51::text,
      51,
      '1'
    )::bit(51)::bigint
  ) AND
  min_altitude_of_bit_string(nodes.bit_string_15) <= (query_z + query_radius) AND
  max_altitude_of_bit_string(nodes.bit_string_15) >= (query_z - query_radius)
);
END;
$BODY$;