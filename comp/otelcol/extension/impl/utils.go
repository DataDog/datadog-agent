package impl

import (
	"os"
	"strings"
)

func getEnvironmentAsMap() map[string]string {
	env := map[string]string{}
	for _, keyVal := range os.Environ() {
		split := strings.SplitN(keyVal, "=", 2)
		key, val := split[0], split[1]
		env[key] = val
	}

	return env
}
