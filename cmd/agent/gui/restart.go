// +build !windows

package gui

import "fmt"

func restart() error {
	return fmt.Errorf("Restarting the agent is not implemented on non-windows platforms.")
}
