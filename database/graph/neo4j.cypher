# Create a unique constraint / index for bit_strings
CREATE CONSTRAINT FOR (n:GeoNode) REQUIRE (n.bit_string) IS UNIQUE

MATCH
  (n:GeoNode {bit_string: "0001000000001001001111110101100"} )-[:LEFT_CHILD|RIGHT_CHILD *0..]->(children),
  n<-[:LEFT_CHILD|RIGHT_CHILD *1..]-(ancestors)

RETURN children,ancestors

MATCH (n:GeoNode {bit_string: "0001000000001001001111110101100"})-[:LEFT_CHILD|RIGHT_CHILD *0..]->(ns)
RETURN DISTINCT ns
UNION
MATCH (n:GeoNode {bit_string: "0001000000001001001111110101100"})<-[:LEFT_CHILD|RIGHT_CHILD *1..]-(ns)
RETURN DISTINCT ns