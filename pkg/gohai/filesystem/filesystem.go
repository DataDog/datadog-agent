// +build linux darwin

package filesystem

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func getFileSystemInfo() (interface{}, error) {
	/* Grab filesystem data from df	*/
	cmd := exec.Command("df", dfOptions...)

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

func parseDfOutput(out string) (interface{}, error) {
	lines := strings.Split(out, "\n")
	var fileSystemInfo []interface{} = make([]interface{}, len(lines)-2)
	for i, line := range lines[1:] {
		values := regexp.MustCompile("\\s+").Split(line, expectedLength)
		if len(values) == expectedLength {
			fileSystemInfo[i] = updatefileSystemInfo(values)
		}
	}
	return fileSystemInfo, nil
}
