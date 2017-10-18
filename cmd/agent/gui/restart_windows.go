package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// restarts the agent uding the windows service manager
func restart() error {
	cmd := exec.Command(filepath.Join(_here, "agent"), "restart-service")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to restart the agent. Error: %v", err)
	}

	return nil
}
