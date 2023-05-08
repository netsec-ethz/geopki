-- Table: nodes

-- DROP TABLE IF EXISTS nodes;

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string bit varying(66) NOT NULL,
    bit_string_52 bigint NOT NULL,
    neighbor_hash bytea,
    left_child_hash bytea,
    right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_pkey PRIMARY KEY (bit_string) -- optional
);

CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx
    ON nodes USING btree
    (bit_string ASC NULLS LAST);

CREATE INDEX IF NOT EXISTS bit_string_integer_idx
    ON nodes USING btree
    (bit_string_52 ASC NULLS LAST);

ALTER TABLE IF EXISTS nodes
    CLUSTER ON bit_string_integer_idx;

CLUSTER nodes USING bit_string_integer_idx;

-- how to convert from nodes-spatial

ALTER TABLE IF EXISTS public.nodes ADD COLUMN bit_string_52 bigint;

UPDATE nodes
SET bit_string_52 =	rpad(
		SUBSTRING(bit_string FROM 1 FOR 52)::text,
		52,
		'0'
	)::bit(52)::bigint

CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx
    ON nodes USING btree
    (bit_string ASC NULLS LAST);

CREATE INDEX IF NOT EXISTS bit_string_integer_idx
    ON nodes USING btree
    (bit_string_52 ASC NULLS LAST);

ALTER TABLE IF EXISTS nodes
    CLUSTER ON bit_string_integer_idx;

CLUSTER nodes USING bit_string_integer_idx;

-- sample queries

SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= 3475785743814656 AND
  bit_string_52 < 3475785743818752

SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= 0 AND
  bit_string_52 < 52 AND
  min_altitude_of_bit_string(bit_string_15) <= 22777 AND
  max_altitude_of_bit_string(bit_string_15) >= 22757


SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= (
    -- 3475785743814656, int("1100010110010011010101101110100100110101".ljust(52, '0'),2)
    rpad(
      '1100010110010011010101101110100100110101',
      52,
      '0'
    )::bit(52)::bigint
  ) AND
  bit_string_52 < (
    -- 3475785743818752, int(bin(int("1100010110010011010101101110100100110101",2)+1)[2:].ljust(52, '0'),2), int("1100010110010011010101101110100100110101".ljust(52, '1'),2)+1
    rpad(
      (
        b'1100010110010011010101101110100100110101'::bigint + 1
        -- has to be a constant, LENGTH(b'01..') does not work
      )::bit(40),
      52,
      '0'
    )::bit(52)::bigint
  )


SELECT * FROM query_by_bitstring(
	b'1100010110010011010101101110100100110101',
	22767::smallint,
	10
)