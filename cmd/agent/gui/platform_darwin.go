package gui

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token string) error {
	confFile, _ := os.Stat(config.Datadog.GetString("conf_path"))
	permissions := confFile.Mode()

	return ioutil.WriteFile(authTokenPath, []byte(token), permissions)
}
