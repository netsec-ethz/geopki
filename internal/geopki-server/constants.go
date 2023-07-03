package server

const (
	// maximum merge delay in seconds
	MAXIMUM_MERGE_DELAY = 5

	// the factor f for new certificates
	F_GROW = 0.1

	// keys in the db 'state' table
	DATABASE_STATE_KEY_DIRTY = "dirty"
)

// list of proxies trusted for HTTP proxy headers
var TRUSTED_PROXIES = []string{"localhost"}
