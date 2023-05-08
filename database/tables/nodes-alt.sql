-- Table: public.nodes

-- DROP TABLE IF EXISTS public.nodes;

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string character varying(66) COLLATE "C" NOT NULL,
    neighbor_hash bytea,
    left_child_hash bytea,
    right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_pkey PRIMARY KEY (bit_string)
)

TABLESPACE pg_default;

ALTER TABLE IF EXISTS public.nodes
    OWNER to postgis_user;

-- create index for more efficient pattern matching
-- https://dba.stackexchange.com/a/291250
CREATE INDEX nodes_bit_string_text_pattern_ops_idx ON nodes(bit_string COLLATE "C");

-- Cluster the rows based on the index
ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_bit_string_text_pattern_ops_idx;
-- CLUSTER nodes USING nodes_bit_string_text_pattern_ops_idx;

-- Conversion from nodes to nodes-alt
ALTER TABLE nodes ADD bit_string_txt character varying(66);
UPDATE nodes SET bit_string_txt = TEXT(bit_string)::character varying(66);
ALTER TABLE nodes DROP CONSTRAINT nodes_pkey;
ALTER TABLE nodes RENAME COLUMN bit_string TO bit_string_bits;
ALTER TABLE nodes RENAME COLUMN bit_string_txt TO bit_string;
ALTER TABLE nodes ADD CONSTRAINT nodes_pkey PRIMARY KEY (bit_string);
CREATE INDEX nodes_bit_string_text_pattern_ops_idx ON nodes(bit_string COLLATE "C");
ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_bit_string_text_pattern_ops_idx;
CLUSTER nodes USING nodes_bit_string_text_pattern_ops_idx;

-- Conversion from nodes-alt to nodes
DROP INDEX nodes_bit_string_text_pattern_ops_idx;
ALTER TABLE nodes ADD bit_string_bits character varying(66);
UPDATE nodes SET bit_string_bits = bit_string::bit(66);
ALTER TABLE nodes DROP CONSTRAINT nodes_pkey;
ALTER TABLE nodes RENAME COLUMN bit_string TO bit_string_txt;
ALTER TABLE nodes RENAME COLUMN bit_string_bits TO bit_string;
ALTER TABLE nodes ADD CONSTRAINT nodes_pkey PRIMARY KEY (bit_string);
CREATE INDEX IF NOT EXISTS nodes_area_geom_idx ON public.nodes USING gist(area) TABLESPACE pg_default;
ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_area_geom_idx;
CLUSTER nodes USING nodes_area_geom_idx;

-- sample query:

SELECT *
	FROM nodes
	WHERE "011101" LIKE bit_string || '%'