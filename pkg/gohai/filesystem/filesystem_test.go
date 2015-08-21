package filesystem

import (
	"fmt"
	"log"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func MockSlowGetFileSystemInfo() (interface{}, error) {
	/* Run a command that will definitely time out */
	cmd := exec.Command("sleep", "5")

	outCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	var out interface{}
	var err error

	go func() {
		_out, _err := cmd.Output()
		if _err != nil {
			errCh <- fmt.Errorf("df failed to collect filesystem data: %s", _err)
			return
		}
		outCh <- _out
	}()

WAIT:
	for {
		select {
		case res := <-outCh:
			if res != nil {
				out, err = parseDfOutput(string(res))
			} else {
				out, err = nil, fmt.Errorf("df failed to collect filesystem data")
			}
			break WAIT
		case err = <-errCh:
			out = nil
			break WAIT
		case <-time.After(2 * time.Second):
			// Kill the process if it takes too long
			if killErr := cmd.Process.Kill(); killErr != nil {
				log.Fatal("failed to kill:", killErr)
				// Force goroutine to exit
				<-outCh
			}
		}
	}

	return out, err
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
