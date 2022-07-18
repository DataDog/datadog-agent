// +build linux darwin

package filesystem

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func withDfCommand(t *testing.T, command ...string) {
	oldCommand := dfCommand
	oldOptions := dfOptions
	dfCommand = command[0]
	if len(command) > 1 {
		dfOptions = command[1:]
	} else {
		dfOptions = []string{}
	}
	t.Cleanup(func() {
		dfCommand = oldCommand
		dfOptions = oldOptions
	})
}

func TestSlowDf(t *testing.T) {
	withDfCommand(t, "sleep", "5")
	dfTimeout = 20 * time.Millisecond // test faster
	defer func() { dfTimeout = 2 * time.Second }()

	_, err := getFileSystemInfo()
	require.ErrorContains(t, err, "df failed to collect filesystem data")
}

func TestFaileDfWithData(t *testing.T) {
	// (note that this sample output is valid on both linux and darwin)
	withDfCommand(t, "sh", "-c", `echo "Filesystem     1K-blocks      Used Available Use% Mounted on"; echo "/dev/disk1s1s1 488245288 138504332 349740956  29% /"; exit 1`)

	out, err := getFileSystemInfo()
	require.NoError(t, err)
	require.Equal(t, []interface{}([]interface{}{
		map[string]string{"kb_size": "488245288", "mounted_on": "/", "name": "/dev/disk1s1s1"},
	}), out)
}

func TestGetFileSystemInfo(t *testing.T) {
	out, err := getFileSystemInfo()
	require.NoError(t, err)
	outArray := out.([]interface{})
	require.Greater(t, len(outArray), 0)
}
