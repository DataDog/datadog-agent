// +build linux darwin

package platform

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

func getArchInfo() (archInfo map[string]interface{}, err error) {
	archInfo = make(map[string]interface{})

	out, err := exec.Command("uname", unameOptions...).Output()
	if err != nil {
		return nil, err
	}
	line := fmt.Sprintf("%s", out)
	values := regexp.MustCompile(" +").Split(line, 7)
	updateArchInfo(archInfo, values)

	out, err = exec.Command("uname", "-v").Output()
	if err != nil {
		return nil, err
	}
	archInfo["kernel_version"] = strings.Trim(string(out), "\n")

	return
}
