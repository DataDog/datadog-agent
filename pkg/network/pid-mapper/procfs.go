package mapper

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func WalkProcFds() error {
	procRoot := util.HostProc()
	d, err := os.Open(procRoot)
	if err != nil {
		return err
	}
	defer d.Close()

	fnames, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, fname := range fnames {
		pid, err := strconv.ParseInt(fname, 10, 32)
		if err != nil {
			// if not numeric name, just skip
			continue
		}

		fdpath := filepath.Join(d.Name(), fname, "fd")
		err = walkFds(int32(pid), fdpath)
		if err != nil {
			continue
		}
	}
	return nil
}

func walkFds(pid int32, path string) error {
	fddir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fddir.Close()

	fdnames, err := fddir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, fdname := range fdnames {
		fdPath := filepath.Join(path, fdname)

		os.Readlink(fdPath)
	}
	return nil
}
