CREATE OR REPLACE FUNCTION prefix_set(input_bit_strings bit varying[])
  RETURNS TABLE (bit_string bit varying)
    LANGUAGE 'plpgsql'
    COST 100
    IMMUTABLE PARALLEL SAFE
AS $BODY$
DECLARE
  input_bit_string bit varying;
BEGIN
FOREACH input_bit_string IN ARRAY input_bit_strings LOOP
  RETURN QUERY(SELECT * FROM prefixes(input_bit_string));
END LOOP;
RETURN QUERY(SELECT b''::bit varying as bit_string);
RETURN;
END;
$BODY$;