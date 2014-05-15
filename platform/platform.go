// +build linux darwin

package platform

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

type Platform struct{}

const name = "platform"

func (self *Platform) Name() string {
	return name
}

func (self *Platform) Collect() (result interface{}, err error) {
	result, err = getPlatformInfo()
	return
}

func getPlatformInfo() (platformInfo map[string]interface{}, err error) {

	platformInfo, err = getArchInfo()
	if err != nil {
		return platformInfo, err
	}

	platformInfo["goV"] = strings.Replace(runtime.Version(), "go", "", -1)
	pythonV, err := getPythonVersion()
	if err != nil {
		return platformInfo, err
	}
	platformInfo["pythonV"] = pythonV

	platformInfo["GOOS"] = runtime.GOOS
	platformInfo["GOOARCH"] = runtime.GOARCH

	return
}

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

func getPythonVersion() (string, error) {
	out, err := exec.Command("python", "-V").CombinedOutput()
	if err != nil {
		return "", err
	}
	version := fmt.Sprintf("%s", out)
	values := regexp.MustCompile("Python (.*)\n").FindStringSubmatch(version)
	return values[1], nil
}
