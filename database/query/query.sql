SELECT
	bit_string_51, bit_string_15, certificate_hashes,
	left_child_hash, right_child_hash, altitude_child_hash
	--, area
FROM nodes
WHERE ST_DWITHIN(
	area,
	'SRID=4326;POINT (8.5413 47.3756)'::geography,
	10
)
AND
(
	min_altitude_of_bit_string(bit_string_15) <= 22777 OR
	max_altitude_of_bit_string(bit_string_15) >= 22757
)

SELECT * FROM query_by_cylinder(
	'SRID=4326;POINT (8.5413 47.3756)'::geography,
	22767::smallint,
	10
)

SELECT *
	FROM nodes
	WHERE "011101" LIKE bit_string || '%'