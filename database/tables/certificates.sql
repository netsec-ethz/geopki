-- Table: public.certificates

-- DROP TABLE IF EXISTS public.certificates;

CREATE TABLE IF NOT EXISTS public.certificates
(
    certificate_hash bytea NOT NULL,
    certificate bytea,
    CONSTRAINT certificates_pkey PRIMARY KEY (certificate_hash)
)

TABLESPACE pg_default;

ALTER TABLE IF EXISTS public.certificates
    OWNER to postgis_user;