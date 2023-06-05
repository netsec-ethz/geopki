CREATE TABLE IF NOT EXISTS certificates
(
    certificate_hash bytea NOT NULL,
    certificate bytea,
    CONSTRAINT certificates_pkey PRIMARY KEY (certificate_hash)
);

CREATE INDEX certificate_hash
    ON public.certificates USING hash
    (certificate_hash)
;