-- FUNCTION: public.max_altitude_of_bit_string(bit varying)

CREATE OR REPLACE FUNCTION public.max_altitude_of_bit_string(
	bit_string bit varying
)
    RETURNS smallint
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
  len integer := LENGTH(bit_string);
	min_altitude smallint := min_altitude_of_bit_string(bit_string);
  altitude_precision int := LENGTH(SUBSTRING(bit_string FROM 52));
BEGIN
RETURN (
    CASE
        -- if the length is less the 51, the full altitude is covered 2^15
        -- note that we subtract one because 2^15 cannot be represented using signed smallint
        -- therefore we shifted the whole range down by 1 from [0, 32'768] to [-1, 32'767]
        WHEN len <= 51 THEN 32767
        -- else add a value on top
        -- could also be computed analogously to 'min_altitude_of_bit_string' but padding with
        -- ones instead. the hope is that the function result is re-used (function is marked as IMMUTABLE)
        ELSE (
          (min_altitude) + (
            1::smallint << (
              15 - altitude_precision
            )
          )
        ) -- times u = 1 = do nothing
    END
);
END;
$BODY$;
