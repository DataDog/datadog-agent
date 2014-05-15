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

	return
}

func getArchInfo() (archInfo map[string]interface{}, err error) {
	archInfo = make(map[string]interface{})

	out, err := exec.Command("uname", "-s", "-n", "-r", "-m", "-p").Output()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	line := fmt.Sprintf("%s", out)
	values := regexp.MustCompile(" +").Split(line, 5)
	archInfo["kernel_name"] = values[0]
	archInfo["hostname"] = values[1]
	archInfo["kernel_release"] = values[2]
	archInfo["machine"] = values[3]
	archInfo["processor"] = strings.Trim(values[4], "\n")
	archInfo["os"] = values[0]

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
