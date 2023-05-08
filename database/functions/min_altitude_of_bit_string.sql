-- FUNCTION: public.min_altitude_of_bit_string(bit varying)

CREATE OR REPLACE FUNCTION public.min_altitude_of_bit_string(
	bit_string bit varying
)
    RETURNS smallint
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
	len integer := LENGTH(bit_string);
BEGIN
RETURN (
    CASE
        -- if the length is less the 51, the full altitude is covered
        WHEN len <= 51 THEN 0
        -- first (right!) pad with 0 to 15 bits, then (left!) pad to 16/32 bits
        -- and then cast it to a smallint.
        -- casting workaround described here https://stackoverflow.com/a/16179288/2897827
        ELSE (
          (
            lpad(
              rpad(
                SUBSTRING(bit_string FROM 52)::text,
                15,
                '0'
              ),
              16,
              '0'
            )::bit(32)::integer
          ) >> 16
        )::smallint -- times u = 1 = do nothing
    END
);
END;
$BODY$;

ALTER FUNCTION public.min_altitude_of_bit_string(bit varying)
    OWNER TO postgis_user;
