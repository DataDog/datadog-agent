package gui

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/kardianos/osext"
)

// restarts the agent using the windows service manager
func restart() error {
	here, _ := osext.ExecutableFolder()
	cmd := exec.Command(filepath.Join(here, "agent"), "restart-service")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to restart the agent. Error: %v", err)
	}

	return nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(jwt, csrf string) error {
	confFile, _ := os.Stat(config.Datadog.GetString("conf_path"))
	permissions := confFile.Mode()
	path := filepath.Join(common.GetDistPath(), "gui_auth_token")

	return ioutil.WriteFile(path, []byte("/authenticate?jwt="+jwt+";csrf="+csrf), permissions)
}
