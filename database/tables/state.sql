CREATE TABLE state
(
    key text NOT NULL,
    value text,
    PRIMARY KEY (key)
);

INSERT INTO state(key,value) VALUES ('dirty', 'false');