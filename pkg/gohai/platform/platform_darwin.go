package platform

import "strings"

var unameOptions = []string{"-s", "-n", "-r", "-m", "-p"}

func updateArchInfo(archInfo map[string]interface{}, values []string) {
	archInfo["kernel_name"] = values[0]
	archInfo["hostname"] = values[1]
	archInfo["kernel_release"] = values[2]
	archInfo["machine"] = values[3]
	archInfo["processor"] = strings.Trim(values[4], "\n")
	archInfo["os"] = values[0]
}
