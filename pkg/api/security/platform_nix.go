// +build freebsd netbsd openbsd solaris dragonfly linux

package security

import (
	"io/ioutil"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token, tokenPath string) error {
	confFile, _ := os.Stat(config.Datadog.ConfigFileUsed())
	permissions := confFile.Mode()

	return ioutil.WriteFile(tokenPath, []byte(token), permissions)
}
