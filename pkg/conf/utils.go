package conf

import "os"

// PathExists returns true if the given path exists
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
