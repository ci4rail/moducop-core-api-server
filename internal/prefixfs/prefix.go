package prefixfs

import "os"

var prefix string

func Path(p string) string {
	return prefix + p
}

func init() {
	// get prefix from CORE_API_SERVER_FS_PREFIX environment variable, default to ""
	prefix = os.Getenv("CORE_API_SERVER_FS_PREFIX")
	if prefix == "" {
		prefix = ""
	}
}
