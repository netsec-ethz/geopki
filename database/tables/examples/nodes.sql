-- sample queries

SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= 3475785743814656 AND
  bit_string_51_int < 3475785743818752

SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= 0 AND
  bit_string_51_int < 52 AND
  min_altitude_of_bit_string(bit_string_15) <= 22777 AND
  max_altitude_of_bit_string(bit_string_15) >= 22757


SELECT bit_string_51_int
FROM nodes
WHERE
  bit_string_51_int >= (
    -- 3475785743814656, int("1100010110010011010101101110100100110101".ljust(51, '0'),2)
    rpad(
      '1100010110010011010101101110100100110101',
      51,
      '0'
    )::bit(51)::bigint
  ) AND
  bit_string_51_int < (
    -- 3475785743818752, int(bin(int("1100010110010011010101101110100100110101",2)+1)[2:].ljust(51, '0'),2), int("1100010110010011010101101110100100110101".ljust(51, '1'),2)+1
    rpad(
      (
        b'1100010110010011010101101110100100110101'::bigint + 1
        -- has to be a constant, LENGTH(b'01..') does not work
      )::bit(40),
      51,
      '0'
    )::bit(51)::bigint
  )