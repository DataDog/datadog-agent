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

func getPythonVersion() (string, error) {
	out, err := exec.Command("python", "-V").CombinedOutput()
	if err != nil {
		return "", err
	}
	version := fmt.Sprintf("%s", out)
	values := regexp.MustCompile("Python (.*)\n").FindStringSubmatch(version)
	return strings.Trim(values[1], "\r"), nil
}
