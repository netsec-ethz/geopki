CREATE OR REPLACE FUNCTION public.hash_of_neighbor(
	input_bit_string_51 bit varying,
	input_bit_string_15 bit varying
)
    RETURNS bytea
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
BEGIN
RETURN (
  CASE
    WHEN LEN(input_bit_string_15) = 0 THEN hash_of_node(
        neighbor_of_bit_string(
          input_bit_string_51
        ),
        input_bit_string_15
      )
    ELSE hash_of_node(
      input_bit_string_51,
      neighbor_of_bit_string(
        input_bit_string_15
      )
    )
  END
);
END;
$BODY$;
