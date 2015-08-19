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

	outCh := make(chan []byte)
	errCh := make(chan error)

	var out interface{}
	var err error

	go func() {
		_out, _err := cmd.Output()
		if _err != nil {
			errCh <- _err
			return
		}
		outCh <- _out
	}()

	select {
	case res := <-outCh:
		if res != nil {
			out, err = parseDfOutput(string(res))
		} else {
			out, err = nil, fmt.Errorf("df process timed out and was killed!")
		}
	case err = <-errCh:
		out = nil
	case <-time.After(2 * time.Second):
		// Kill the process if it takes too long
		if killErr := cmd.Process.Kill(); killErr != nil {
			log.Fatal("failed to kill:", killErr)
		}
		//Let goroutine exit
		<-outCh
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
