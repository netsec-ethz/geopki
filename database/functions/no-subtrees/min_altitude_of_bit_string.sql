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
        -- note that we subtract one because 2^15 cannot be represented using signed smallint
        -- therefore we shifted the whole range down by 1 from [0, 32'768] to [-1, 32'767]
        WHEN len <= 51 THEN -1
        -- first (right!) pad with 0 to 15 bits, then (left!) pad to 16/32 bits
        -- and then cast it to a smallint.
        -- casting workaround described here https://stackoverflow.com/a/16179288/2897827
        -- finally subtract -1 because of the range shift
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
        )::smallint - 1 -- times u = 1 = do nothing
    END
);
END;
$BODY$;
