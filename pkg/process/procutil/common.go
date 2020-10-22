package procutil

import (
	"os"
	"path/filepath"
)

// HostProc returns the procfs location
func HostProc(combineWith ...string) string {
	return GetEnv("HOST_PROC", "/proc", combineWith...)
}

//GetEnv retrieves the environment variable key. If it does not exist it returns the default.
func GetEnv(key string, fallback string, combineWith ...string) string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}

	switch len(combineWith) {
	case 0:
		return value
	case 1:
		return filepath.Join(value, combineWith[0])
	default:
		all := make([]string, len(combineWith)+1)
		all[0] = value
		copy(all[1:], combineWith)
		return filepath.Join(all...)
	}
}

// DoesDirExist tests if the given path exists or not in the file system. It returns a boolean as well as
// any errors that might happen
func DoesDirExist(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
