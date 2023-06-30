CREATE TABLE IF NOT EXISTS certificates
(
    certificate_hash bytea NOT NULL,
    certificate bytea,
    not_valid_after timestamp with time zone NOT NULL DEFAULT '2030-01-01 00:00:00+00',
    CONSTRAINT certificates_pkey PRIMARY KEY (certificate_hash)
);

CREATE INDEX certificate_hash
    ON certificates USING hash
    (certificate_hash)
;

CREATE INDEX certificate_not_valid_after
    ON certificates USING btree
    (not_valid_after)
;

ALTER TABLE IF EXISTS certificates
    CLUSTER ON certificate_not_valid_after;

CLUSTER certificates USING certificate_not_valid_after;