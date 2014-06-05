package platform

import (
	utils "github.com/DataDog/gohai/windowsutils"
)

func getArchInfo() (systemInfo map[string]interface{}, err error) {
	systemInfo = make(map[string]interface{})

	computerSystem, err := utils.WindowsWMICommand("COMPUTERSYSTEM", "Name", "SystemType")
	if err != nil {
		return
	}
	systemInfo["hostname"] = computerSystem["Name"]
	systemInfo["machine"] = computerSystem["SystemType"]

	os, err := utils.WindowsWMICommand("OS", "Version", "Caption")
	if err != nil {
		return
	}
	systemInfo["kernel_release"] = os["Version"]
	systemInfo["os"] = os["Caption"]

	systemInfo["kernel_name"] = "Windows"

	return
}
