CREATE OR REPLACE FUNCTION prefixes(input_bit_string bit varying)
  RETURNS TABLE (bit_string bit varying)
    LANGUAGE 'plpgsql'
    COST 100
    STABLE PARALLEL SAFE
AS $BODY$
DECLARE
  len int := LENGTH(input_bit_string);
BEGIN
FOR i IN 1.. len LOOP
  RETURN QUERY (
    SELECT (SUBSTRING(input_bit_string FROM 1 FOR i)) as bit_string
  );
END LOOP;
RETURN;
END;
$BODY$;