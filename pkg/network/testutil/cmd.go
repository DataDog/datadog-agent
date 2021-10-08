package testutil

import (
	"os/exec"
	"strings"
	"testing"
)

// RunCommands runs each command in cmds individually and returns the output
// as a []string, with each element corresponding to the respective command.
// If ignoreErrors is true, it will fail the test via t.Fatal immediately upon error.
// Otherwise, the output on errors will be logged via t.Log.
func RunCommands(t *testing.T, cmds []string, ignoreErrors bool) []string {
	t.Helper()
	var output []string

	for _, c := range cmds {
		args := strings.Split(c, " ")
		c := exec.Command(args[0], args[1:]...)
		out, err := c.CombinedOutput()
		output = append(output, string(out))
		if err != nil && !ignoreErrors {
			t.Fatalf("%s returned %s: %s", c, err, out)
			return nil
		}
	}
	return output
}
