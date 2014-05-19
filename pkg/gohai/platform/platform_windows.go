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

	platformInfo, err = getSystemInfo()
	if err != nil {
		return nil, err
	}
	platformInfo["goV"] = strings.Replace(runtime.Version(), "go", "", -1)

	pv, err := getPythonVersion()
	if err != nil {
		return
	}
	platformInfo["pythonV"] = pv
	platformInfo["GOOS"] = runtime.GOOS
	platformInfo["GOOARCH"] = runtime.GOARCH

	return
}

var systemMap = map[string]string{
	"Host Name:":   "hostname",
	"OS Name:":     "kernel_name",
	"OS Version:":  "kernel_release",
	"System Type:": "machine",
}

func getSystemInfo() (systemInfo map[string]interface{}, err error) {
	systemInfo = make(map[string]interface{})

	// out, err := exec.Command("wmic.exe", "PARTITION").Output()
	// if err != nil {
	// 	return nil, err
	// }
	// println(string(out))

	// // PLATFORM
	// "COMPUTERSYSTEM"
	// archInfo["hostname"] = "NAME"
	// archInfo["machine"] = "SystemType"

	// "OS"
	// archInfo["kernel_release"] = "Version"
	// archInfo["os"] = "Caption"
	// archInfo["kernel_name"] = ""//"Windows"

	// // MEM
	// "COMPUTERSYSTEM"
	// "TotalPhysicalMemory":  "total",

	// // FS
	// "VOLUME"
	// "name":       "Name",
	// "kb_size":    "Capacity",
	// "mounted_on": "DriveLetter",

	// // CPU
	// "CPU"
	// "cpu family": "Family",
	// "cpu MHz\t":  "CurrentClockSpeed",
	// "model name": "Name",
	// "cpu cores":  "NumberOfCores",
	// "stepping":   "Split Caption",
	// "model\t":    "Split Caption",
	// "vendor_id":  "Manufacturer",

	// lines := strings.Split(string(out), "\n")

	// for i, line := range lines[1:] {
	// 	values := regexp.MustCompile("  +").Split(line, 30)
	// 	if len(values) == 2 {
	// 		key, ok := systemMap[values[0]]
	// 		if ok {
	// 			systemInfo[key] = strings.Trim(values[1], "\r")
	// 		} else if values[0] == "Processor(s):" {
	// 			processor := strings.Split(lines[i+2], "[01]: ")[1]
	// 			systemInfo["processor"] = strings.Trim(processor, "\r")
	// 		}
	// 	}
	// 	systemInfo["os"] = systemInfo["kernel_name"]
	// }

	return
}

func getPythonVersion() (string, error) {
	out, err := exec.Command("python", "-V").CombinedOutput()
	if err != nil {
		return "", err
	}
	version := fmt.Sprintf("%s", out)
	values := regexp.MustCompile("Python (.*)\r\n").FindStringSubmatch(version)
	return values[1], nil
}
