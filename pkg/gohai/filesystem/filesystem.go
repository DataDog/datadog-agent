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

func getFileSystemInfo() (fileSystemInfo map[string]interface{}, err error) {

	fileSystemInfo = make(map[string]interface{})

	/* Grab filesystem data from df	*/
	out, err := exec.Command("df", dfOptions...).Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		values := regexp.MustCompile("\\s+").Split(line, expectedLength)
		if len(values) == expectedLength {
			updatefileSystemInfo(fileSystemInfo, values)
		}
	}

	return
}
