-- FUNCTION: public.hash_of_neighbor(bit varying)

CREATE OR REPLACE FUNCTION public.hash_of_neighbor(
	bit_string bit varying
)
    RETURNS bytea
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
BEGIN
RETURN hash_of_node(
  neighbor_of_bit_string(
    bit_string
  )
);
END;
$BODY$;

ALTER FUNCTION public.hash_of_neighbor(bit varying)
    OWNER TO postgis_user;
