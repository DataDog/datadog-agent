package common

import (
	"os"
	"path/filepath"
)

func HostProc(combineWith ...string) string {
	return GetEnv("HOST_PROC", "/proc", combineWith...)
}

func HostSys(combineWith ...string) string {
	return GetEnv("HOST_SYS", "/sys", combineWith...)
}

func HostEtc(combineWith ...string) string {
	return GetEnv("HOST_ETC", "/etc", combineWith...)
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

func DoesDirExist(filepath string) bool {
	// TODO (SK): There's probably a better way of checking if a directory exists...
	file, err := os.Open(filepath)
	file.Close()
	return err == nil
}
