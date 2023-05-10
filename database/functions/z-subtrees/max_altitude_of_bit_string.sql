CREATE OR REPLACE FUNCTION public.max_altitude_of_bit_string(
	bit_string_15 bit varying
)
    RETURNS smallint
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
	len integer := LENGTH(bit_string_15);
BEGIN
RETURN (
    CASE
        -- if the length is 0, the full altitude is covered
        WHEN len = 0 THEN 0
        -- first (right!) pad with 1 to 15 bits, then (left!) pad to 16/32 bits
        -- and then cast it to a smallint.
        -- casting workaround described here https://stackoverflow.com/a/16179288/2897827
        ELSE (
          (
            lpad(rpad(bit_string_15, 15, '1' ),16,'0')::bit(32)::integer
          ) >> 16
        )::smallint -- times u = 1 = do nothing
    END
);
END;
$BODY$;
