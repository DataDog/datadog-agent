// +build linux darwin

package filesystem

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func MockSlowGetFileSystemInfo() (interface{}, error) {
	/* Run a command that will definitely time out */
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	/* Grab filesystem data from df	*/
	cmd := exec.CommandContext(ctx, "sleep", "5")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("df failed to collect filesystem data: %s", err)
	}
	if out != nil {
		return parseDfOutput(string(out))
	}
	return nil, fmt.Errorf("df failed to collect filesystem data")
}

func TestSlowGetFileSystemInfo(t *testing.T) {
	out, err := MockSlowGetFileSystemInfo()
	if !reflect.DeepEqual(out, nil) {
		t.Fatalf("Failed! out should be nil. Instead it's %s", out)
	}
	if !reflect.DeepEqual(err, fmt.Errorf("df failed to collect filesystem data: signal: killed")) {
		t.Fatalf("Failed! Wrong error: %s", err)
	}
}

func TestGetFileSystemInfo(t *testing.T) {
	_, err := getFileSystemInfo()
	if !reflect.DeepEqual(err, nil) {
		t.Fatalf("getFileSystemInfo failed: %s", err)
	}
}
