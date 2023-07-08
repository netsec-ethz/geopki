CREATE OR REPLACE FUNCTION query_by_bitstrings(
	query_bit_strings bit varying(51)[],
	min_altitude smallint,
	max_altitude smallint
)
  RETURNS TABLE (
		bit_string_51 bit varying(51),
		bit_string_15 bit varying(15),
		xy_left_child_hash bytea,
		xy_right_child_hash bytea,
		z_left_child_hash bytea,
		z_right_child_hash bytea,
		certificate_hashes bytea[]
	)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  query_bit_string bit varying;
BEGIN
FOREACH query_bit_string IN ARRAY query_bit_strings LOOP
  RETURN QUERY (
    SELECT
      nodes.bit_string_51,
      nodes.bit_string_15,
      nodes.xy_left_child_hash,
      nodes.xy_right_child_hash,
      nodes.z_left_child_hash,
      nodes.z_right_child_hash,
      nodes.certificate_hashes
    FROM nodes
    WHERE bit_string_51_int >= (rpad(query_bit_string::text,51,'0')::bit(51)::bigint)
    AND bit_string_51_int <= (rpad(query_bit_string::text,51,'1')::bit(51)::bigint)
    AND altitude_min <= max_altitude
    AND altitude_max >= min_altitude
  );
END LOOP;
RETURN QUERY (
  SELECT
    nodes.bit_string_51,
    nodes.bit_string_15,
    nodes.xy_left_child_hash,
    nodes.xy_right_child_hash,
    nodes.z_left_child_hash,
    nodes.z_right_child_hash,
    nodes.certificate_hashes
  FROM nodes
  WHERE nodes.bit_string_51 IN (SELECT DISTINCT bit_string FROM prefix_set(query_bit_strings))
  AND altitude_min <= max_altitude
  AND altitude_max >= min_altitude
);
RETURN;
END;
$BODY$;