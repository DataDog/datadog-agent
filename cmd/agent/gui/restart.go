// +build !windows

package gui

import "fmt"

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}
