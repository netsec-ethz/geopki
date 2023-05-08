-- FUNCTION: public.right_child_of_bit_string(bit varying)

CREATE OR REPLACE FUNCTION public.right_child_of_bit_string(
	bit_string bit varying
)
    RETURNS bit varying
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
	len integer := LENGTH(bit_string);
BEGIN
RETURN (
    CASE
        -- 66 is the maximum depth, this is a leaf node and does
        -- not have any children
        WHEN len >= 66 THEN NULL
        ELSE bit_string || b'1'
    END
);
END;
$BODY$;

ALTER FUNCTION public.right_child_of_bit_string(bit varying)
    OWNER TO postgis_user;
