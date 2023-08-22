-- Table: nodes

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string_51 bit varying(51) NOT NULL,
    bit_string_51_int bigint NOT NULL GENERATED ALWAYS AS (rpad(SUBSTRING(bit_string_51 FROM 1 FOR 51)::text,51,'0')::bit(51)::bigint) STORED,
    bit_string_15 bit varying(15) NOT NULL,
    altitude_min smallint NOT NULL GENERATED ALWAYS AS (min_altitude_of_bit_string(bit_string_15)) STORED,
    altitude_max smallint NOT NULL GENERATED ALWAYS AS (max_altitude_of_bit_string(bit_string_15)) STORED,
    xy_left_child_hash bytea,
    xy_right_child_hash bytea,
    z_left_child_hash bytea,
    z_right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[]
);

CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx
    ON nodes USING btree
    -- ON nodes USING hash
    (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);

CREATE INDEX IF NOT EXISTS bit_string_integer_idx
    ON nodes USING btree
    (bit_string_51_int ASC NULLS LAST);

ALTER TABLE IF EXISTS nodes CLUSTER ON bit_string_integer_idx;
CLUSTER nodes USING bit_string_integer_idx;

-- VACUUM FULL nodes;

-- create copy of nodes called 'nodes_next', contains the data for the next version
CREATE TABLE IF NOT EXISTS nodes_next
(
    bit_string_51 bit varying(51) NOT NULL,
    bit_string_51_int bigint NOT NULL GENERATED ALWAYS AS (rpad(SUBSTRING(bit_string_51 FROM 1 FOR 51)::text,51,'0')::bit(51)::bigint) STORED,
    bit_string_15 bit varying(15) NOT NULL,
	-- note we shift the range down by 1 from [0, 32'768] to [-1, 32'767] because 2^15 cannot be represented using signed smallint
    -- (and there are no unsigned smallints)
    altitude_min smallint NOT NULL GENERATED ALWAYS AS (min_altitude_of_bit_string(bit_string_15)) STORED,
    altitude_max smallint NOT NULL GENERATED ALWAYS AS (max_altitude_of_bit_string(bit_string_15)) STORED,
    xy_left_child_hash bytea,
    xy_right_child_hash bytea,
    z_left_child_hash bytea,
    z_right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_next_pkey PRIMARY KEY (bit_string_51, bit_string_15)
);
INSERT INTO nodes_next(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) SELECT bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes FROM nodes;