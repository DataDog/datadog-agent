package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
