CREATE OR REPLACE FUNCTION public.query_by_cylinder(
	query_point geography,
	query_z smallint,
	query_radius integer
)
    RETURNS TABLE (
		bit_string_51 bit varying(51),
    bit_string_15 bit varying(15),
		certificate_hashes bytea[],
		xy_left_child_hash bytea,
    xy_right_child_hash bytea,
    z_left_child_hash bytea,
    z_right_child_hash bytea
		--, area geography
	)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
BEGIN
RETURN QUERY (
  SELECT
    nodes.bit_string_51, nodes.bit_string_15,
    nodes.xy_left_child_hash, nodes.xy_right_child_hash,
    nodes.z_left_child_hash, nodes.z_right_child_hash,
    nodes.certificate_hashes
    --, area
  FROM nodes
  WHERE ST_DWITHIN(
    area,
    query_point,
    query_radius
  ) AND
  min_altitude_of_bit_string(nodes.bit_string) <= (query_z + query_radius) AND
  max_altitude_of_bit_string(nodes.bit_string) >= (query_z - query_radius)
);
END;
$BODY$;
