-- Table: nodes

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string_51 bit varying(51) NOT NULL,
    area geography NOT NULL,
    bit_string_15 bit varying(15) NOT NULL,
    altitude_min smallint NOT NULL GENERATED ALWAYS AS (min_altitude_of_bit_string(bit_string_15)) STORED,
    altitude_max smallint NOT NULL GENERATED ALWAYS AS (max_altitude_of_bit_string(bit_string_15)) STORED,
    xy_left_child_hash bytea,
    xy_right_child_hash bytea,
    z_left_child_hash bytea,
    z_right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_pkey PRIMARY KEY (bit_string_51, bit_string_15)
);

CREATE INDEX IF NOT EXISTS bit_string_len ON nodes (LENGTH(bit_string_51), LENGTH(bit_string_15));

CREATE INDEX IF NOT EXISTS nodes_area_geom_idx ON public.nodes USING gist (area);
ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_area_geom_idx;
CLUSTER nodes USING nodes_area_geom_idx;