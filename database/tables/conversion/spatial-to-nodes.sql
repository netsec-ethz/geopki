ALTER TABLE IF EXISTS public.nodes ADD COLUMN bit_string_51_int bigint;

UPDATE nodes
SET bit_string_51_int =	rpad(
		SUBSTRING(bit_string FROM 1 FOR 51)::text,
		51,
		'0'
	)::bit(51)::bigint

UPDATE nodes SET bit_string_51 = SUBSTRING(bit_string FROM 1 FOR 51)::bit varying(51)
UPDATE nodes SET altitude_min = min_altitude_of_bit_string(bit_string_15), altitude_max = max_altitude_of_bit_string(bit_string_15)

CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx
    ON nodes USING btree
    (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST)
;

CREATE INDEX IF NOT EXISTS bit_string_integer_idx
    ON nodes USING btree
    (bit_string_51_int ASC NULLS LAST);

ALTER TABLE IF EXISTS nodes
    CLUSTER ON bit_string_integer_idx;

CLUSTER nodes USING bit_string_integer_idx;