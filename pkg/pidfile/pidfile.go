package pidfile

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WritePID writes the current PID to a file, ensuring that the file
// doesn't exist or doesn't contain a PID for a running process. If
// WritePID is invoked without params, Path() is used to determine
// the pidfile path
func WritePID(path ...string) error {
	var pidFilePath string
	// default value
	if len(path) == 0 {
		pidFilePath = Path()
	} else {
		pidFilePath = path[0]
	}

	// check whether the pidfile exists and contains the PID for a running proc...
	exists := false
	if byteContent, err := ioutil.ReadFile(pidFilePath); err == nil {
		pidStr := strings.TrimSpace(string(byteContent))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			exists = isProcess(pid)
		}
	}

	// ...and return an error in case
	if exists {
		return fmt.Errorf("Pidfile already exists, please check %s isn't running or remove %s",
			os.Args[0], pidFilePath)
	}

	// create the full path to the pidfile
	if err := os.MkdirAll(filepath.Dir(pidFilePath), os.FileMode(0755)); err != nil {
		return err
	}

	// write current pid in it
	pidStr := fmt.Sprintf("%d", os.Getpid())
	if err := ioutil.WriteFile(pidFilePath, []byte(pidStr), 0644); err != nil {
		return err
	}

	// all good
	return nil
}
