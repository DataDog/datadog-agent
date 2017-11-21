// +build freebsd netbsd openbsd solaris dragonfly linux

package gui

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(jwt, csrf string) error {
	confFile, _ := os.Stat(config.Datadog.GetString("conf_path"))
	permissions := confFile.Mode()
	path := filepath.Join(common.GetDistPath(), "gui_auth_token")

	return ioutil.WriteFile(path, []byte("/authenticate?jwt="+jwt+";csrf="+csrf), permissions)
}
