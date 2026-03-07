package prefixfs

import "os"

var prefix string

func Path(p string) string {
	return prefix + p
}

func init() {
	// get prefix from MOCK_MENDER_STATE_DIR environment variable, default to ""
	// append "/fs" to the prefix if it is set, so that the final path will be "<prefix>/fs/<path>"
	prefix = os.Getenv("MOCK_MENDER_STATE_DIR")
	if prefix != "" {
		prefix = prefix + "/fs"
	}
}
