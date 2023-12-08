//go:build windows

package command

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestFxRunCommand(t *testing.T) {
	cmd := MakeCommand()
	fxutil.TestRun(t, func() error {
		return cmd.Execute()
	})
}
