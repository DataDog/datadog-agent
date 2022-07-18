// +build linux darwin

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

var dfCommand = "df"
var dfTimeout = 2 * time.Second

func getFileSystemInfo() (interface{}, error) {

	ctx, cancel := context.WithTimeout(context.Background(), dfTimeout)
	defer cancel()

	/* Grab filesystem data from df	*/
	cmd := exec.CommandContext(ctx, dfCommand, dfOptions...)

	out, execErr := cmd.Output()
	var parseErr error
	var result []interface{}
	if out != nil {
		result, parseErr = parseDfOutput(string(out))
	}

	// if we managed to get _any_ data, just use it, ignoring other errors
	if result != nil && len(result) != 0 {
		return result, nil
	}

	// otherwise, prefer the parse error, as it is probably more detailed
	err := execErr
	if parseErr != nil {
		err = parseErr
	}
	if err == nil {
		err = errors.New("unknown error")
	}
	return nil, fmt.Errorf("df failed to collect filesystem data: %s", parseErr)
}

func parseDfOutput(out string) ([]interface{}, error) {
	var aggregateError error
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return nil, errors.New("no output")
	}
	var fileSystemInfo = make([]interface{}, 0, len(lines)-2)
	for _, line := range lines[1:] {
		values := regexp.MustCompile(`\s+`).Split(line, -1)
		if len(values) >= expectedLength {
			info, err := updatefileSystemInfo(values)
			if err != nil {
				aggregateError = multierror.Append(aggregateError, err)
			} else {
				fileSystemInfo = append(fileSystemInfo, info)
			}
		}
	}
	return fileSystemInfo, aggregateError
}
