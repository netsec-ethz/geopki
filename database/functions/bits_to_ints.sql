ALTER TABLE IF EXISTS public.nodes ADD COLUMN bit_string_52 bigint;
ALTER TABLE IF EXISTS public.nodes ADD PRIMARY KEY (bit_string_52);

UPDATE nodes
SET bit_string_52 =	rpad(
		SUBSTRING(bit_string FROM 1 FOR 52)::text,
		52,
		'0'
	)::bit(52)::bigint,
	bit_string_15 = SUBSTRING(bit_string FROM 52)::bit varying(15)


SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= 3475785743814656 AND
  bit_string_52 < 3475785743818752

SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= 0 AND
  bit_string_52 < 52 AND
  min_altitude_of_bit_string(bit_string_15) <= 22777 AND
  max_altitude_of_bit_string(bit_string_15) >= 22757


SELECT bit_string_52
FROM nodes
WHERE
  bit_string_52 >= (
    -- 3475785743814656, int("1100010110010011010101101110100100110101".ljust(52, '0'),2)
    rpad(
      '1100010110010011010101101110100100110101',
      52,
      '0'
    )::bit(52)::bigint
  ) AND
  bit_string_52 < (
    -- 3475785743818752, int(bin(int("1100010110010011010101101110100100110101",2)+1)[2:].ljust(52, '0'),2), int("1100010110010011010101101110100100110101".ljust(52, '1'),2)+1
    rpad(
      (
        b'1100010110010011010101101110100100110101'::bigint + 1
        -- has to be a constant, LENGTH(b'01..') does not work
      )::bit(40),
      52,
      '0'
    )::bit(52)::bigint
  )

CREATE INDEX bit_string_integer_idx ON public.nodes USING btree (bit_string_52 ASC NULLS LAST);
ALTER TABLE IF EXISTS public.nodes CLUSTER ON bit_string_integer_idx;
CLUSTER nodes USING bit_string_integer_idx;

CREATE UNIQUE INDEX bit_string_bit_idx
    ON public.nodes USING btree
    (bit_string ASC NULLS LAST)
;