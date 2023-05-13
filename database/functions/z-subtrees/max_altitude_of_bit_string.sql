CREATE OR REPLACE FUNCTION public.max_altitude_of_bit_string(
	bit_string_15 bit varying
)
    RETURNS smallint
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
BEGIN
-- first (right!) pad with 1 to 15 bits, then (left!) pad to 16/32 bits
-- and then cast it to a smallint.
-- casting workaround described here https://stackoverflow.com/a/16179288/2897827
RETURN (
  (
    lpad(rpad(bit_string_15::text, 15, '1'),16,'0')::bit(32)::integer
  ) >> 16
)::smallint; -- times u = 1 = do nothing
END;
$BODY$;
