// +build !windows

package gui

import (
	"fmt"
	"os"
	"os/exec"
)

// restarts the agent
func restart() error {
	cmd := exec.Command("sudo service datadog-agent restart")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to restart process. Error: %v", err)
	}

	return nil
}
