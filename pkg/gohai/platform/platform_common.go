// +build !android

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

	// collect each portion, and allow the parts that succeed (even if some
	// parts fail.)  For this check, it does have the (small) liability
	// that if both the ArchInfo() and the PythonVersion() fail, the error
	// from the ArchInfo() will be lost

	// for this, no error check.  The successful results will be added
	// to the return value, and the error stored.
	platformInfo, err = GetArchInfo()
	if platformInfo == nil {
		platformInfo = make(map[string]interface{})
	}

	platformInfo["goV"] = strings.Replace(runtime.Version(), "go", "", -1)
	// If this errors, swallow the error.
	// It will usually mean that Python is not on the PATH
	// and we don't care about that.
	pythonV, e := getPythonVersion()

	// if there was no failure, add the python variables to the platformInfo
	if e == nil {
		platformInfo["pythonV"] = pythonV
	}

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
