-- Table: public.nodes

-- DROP TABLE IF EXISTS public.nodes;

CREATE TABLE IF NOT EXISTS nodes
(
    bit_string bit varying(66) NOT NULL,
    area geography NOT NULL,
    left_child_hash bytea,
    right_child_hash bytea,
    certificate_hashes bytea[] NOT NULL DEFAULT '{}'::bytea[],
    CONSTRAINT nodes_pkey PRIMARY KEY (bit_string)
)

TABLESPACE pg_default;

ALTER TABLE IF EXISTS public.nodes
    OWNER to postgis_user;
-- Index: nodes_area_geom_idx

-- DROP INDEX IF EXISTS public.nodes_area_geom_idx;

CREATE INDEX IF NOT EXISTS nodes_area_geom_idx
    ON public.nodes USING gist
    (area)
    TABLESPACE pg_default;

-- Cluster the rows based on their spatial index

ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_area_geom_idx;
CLUSTER nodes USING nodes_area_geom_idx;