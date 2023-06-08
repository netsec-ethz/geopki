CREATE TABLE IF NOT EXISTS certificates
(
    certificate_hash bytea NOT NULL,
    certificate bytea,
    not_valid_after timestamp with time zone NOT NULL DEFAULT '2030-01-01 00:00:00+00',
    CONSTRAINT certificates_pkey PRIMARY KEY (certificate_hash)
);

CREATE INDEX certificate_hash
    ON public.certificates USING hash
    (certificate_hash)
;