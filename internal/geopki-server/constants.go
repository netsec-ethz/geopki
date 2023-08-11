package server

const (
	// maximum merge delay in seconds (at the moment only relevant for the trillian based consistency tree)
	MAXIMUM_MERGE_DELAY = 5

	// the factor f for new certificates
	F_GROW = 0.1
)
