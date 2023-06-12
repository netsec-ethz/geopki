-- FUNCTION: public.neighbor_of_bit_string(bit varying)

CREATE OR REPLACE FUNCTION public.neighbor_of_bit_string(
	bit_string bit varying
)
    RETURNS bit varying
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
	len integer := LENGTH(bit_string);
	last_bit bit := get_bit(bit_string, len-1);
BEGIN

RETURN SUBSTRING(bit_string FROM 1 FOR len-1) || (~last_bit);

END;
$BODY$;

ALTER FUNCTION public.neighbor_of_bit_string(bit varying)
    OWNER TO postgis_user;
