-- Table: nodes

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string_51 bit varying(51) NOT NULL,
    bit_string_51_txt character varying(51) COLLATE "C" NOT NULL GENERATED ALWAYS AS (bit_string_51::character varying(51)) STORED,
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

CREATE INDEX IF NOT EXISTS bit_string_len ON nodes (LENGTH(bit_string_51), LENGTH(bit_string_15));
-- CREATE INDEX bit_string_len
--     ON nodes USING hash
--     (LENGTH(bit_string_51), LENGTH(bit_string_15));

-- create index for more efficient pattern matching
-- https://dba.stackexchange.com/a/291250
CREATE INDEX nodes_bit_string_text_pattern_ops_idx ON nodes(bit_string_51_txt COLLATE "C");

ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_bit_string_text_pattern_ops_idx;

CLUSTER nodes USING nodes_bit_string_text_pattern_ops_idx;

-- VACUUM FULL nodes;