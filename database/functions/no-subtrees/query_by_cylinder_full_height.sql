CREATE OR REPLACE FUNCTION public.query_by_cylinder_full_height(
	query_point geography,
	query_z smallint,
	query_radius integer
)
    RETURNS TABLE (
		bit_string bit varying(66),
		certificate_hashes bytea[],
		left_child_hash bytea,
		right_child_hash bytea
		--, area geography
	)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
BEGIN
RETURN QUERY (
  SELECT
    nodes.bit_string, nodes.certificate_hashes,
    nodes.left_child_hash, nodes.right_child_hash
    --, area
  FROM nodes
  WHERE ST_DWITHIN(
    area,
    query_point,
    query_radius
  ) AND
  min_altitude_of_bit_string(nodes.bit_string) <= 32767 AND
  max_altitude_of_bit_string(nodes.bit_string) >= 0
);
END;
$BODY$;

ALTER FUNCTION public.query_by_cylinder(geography, smallint, integer)
    OWNER TO postgis_user;
