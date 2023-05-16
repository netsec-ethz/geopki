-- Table: nodes

-- DROP TABLE IF EXISTS nodes;

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string_51 bit varying(51) NOT NULL,
    bit_string_51_int bigint NOT NULL,
    bit_string_15 bit varying(15) NOT NULL,
    altitude_min smallint NOT NULL DEFAULT 0,
    altitude_max smallint NOT NULL DEFAULT 32767,
    neighbor_hash bytea,
    xy_left_child_hash bytea,
    xy_right_child_hash bytea,
    z_left_child_hash bytea,
    z_right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_pkey PRIMARY KEY (bit_string_51, bit_string_15) -- optional
);

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

-- how to convert from nodes-spatial

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

-- sample queries

SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= 3475785743814656 AND
  bit_string_51_int < 3475785743818752

SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= 0 AND
  bit_string_51_int < 52 AND
  min_altitude_of_bit_string(bit_string_15) <= 22777 AND
  max_altitude_of_bit_string(bit_string_15) >= 22757


SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= (
    -- 3475785743814656, int("1100010110010011010101101110100100110101".ljust(51, '0'),2)
    rpad(
      '1100010110010011010101101110100100110101',
      51,
      '0'
    )::bit(51)::bigint
  ) AND
  bit_string_51_int < (
    -- 3475785743818752, int(bin(int("1100010110010011010101101110100100110101",2)+1)[2:].ljust(51, '0'),2), int("1100010110010011010101101110100100110101".ljust(51, '1'),2)+1
    rpad(
      (
        b'1100010110010011010101101110100100110101'::bigint + 1
        -- has to be a constant, LENGTH(b'01..') does not work
      )::bit(40),
      51,
      '0'
    )::bit(51)::bigint
  )


SELECT * FROM query_by_bitstring(
	b'1100010110010011010101101110100100110101',
	22767::smallint,
	10
)