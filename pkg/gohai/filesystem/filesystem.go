package filesystem

import (
	"os/exec"
	"regexp"
	"strings"
)

type FileSystem struct{}

const name = "filesystem"

func (self *FileSystem) Name() string {
	return name
}

func (self *FileSystem) Collect() (result interface{}, err error) {
	result, err = getFileSystemInfo()
	return
}

func getFileSystemInfo() (interface{}, error) {

	/* Grab filesystem data from df	*/
	out, err := exec.Command("df", dfOptions...).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var fileSystemInfo []interface{} = make([]interface{}, len(lines)-2)
	for i, line := range lines[1:] {
		values := regexp.MustCompile("\\s+").Split(line, expectedLength)
		if len(values) == expectedLength {
			fileSystemInfo[i] = updatefileSystemInfo(values)
		}
	}

	return fileSystemInfo, nil
}
