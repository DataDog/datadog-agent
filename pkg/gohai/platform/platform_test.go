package platform

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/gohai/utils"
)

func TestParsePythonVersion(t *testing.T) {
	t.Run("valid Python version", func(t *testing.T) {
		pythonV, err := parsePythonVersion([]byte("Python 3.8.9\n"))
		assert.Nil(t, err)
		assert.Equal(t, "3.8.9", pythonV)
	})

	t.Run("valid Python version (windows)", func(t *testing.T) {
		pythonV, err := parsePythonVersion([]byte("Python 3.8.9\r\n"))
		assert.Nil(t, err)
		assert.Equal(t, "3.8.9", pythonV)
	})

	t.Run("invalid Python version", func(t *testing.T) {
		pythonV, err := parsePythonVersion([]byte("gibberish"))
		assert.NotNil(t, err)
		assert.Equal(t, "", pythonV)
	})
}

func TestGetPythonVersion(t *testing.T) {
	t.Run("valid command", func(t *testing.T) {
		pythonV, err := getPythonVersion(utils.BuildFakeExecCmd("TestGetPythonVersionCmd", "valid-command"))
		assert.Nil(t, err)
		assert.Equal(t, "3.8.9", pythonV)
	})

	t.Run("Python not found", func(t *testing.T) {
		pythonV, err := getPythonVersion(utils.BuildFakeExecCmd("TestGetPythonVersionCmd", "python-not-found"))
		assert.NotNil(t, err)
		assert.Equal(t, "", pythonV)
	})
}

// TestGetHostnameShellCmd is a method that is called as a substitute for a shell command by the
// fake ExecCmd built with utils.BuildFakeExecCmd("TestGetPythonVersionCmd", "foo").
// The GO_TEST_PROCESS flag ensures that if it is called as part of the test suite, it is skipped.
func TestGetPythonVersionCmd(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}

	testRunName, cmdList := utils.ParseFakeExecCmdArgs()

	assert.EqualValues(t, []string{"python", "-V"}, cmdList)

	switch testRunName {
	case "valid-command":
		fmt.Fprintf(os.Stdout, "Python 3.8.9\n")
		os.Exit(0)
	case "python-not-found":
		fmt.Fprintf(os.Stdout, "")
		fmt.Fprintf(os.Stderr, "command not found: python")
		os.Exit(127)
	}
}
