package memory

import (
	utils "github.com/DataDog/gohai/windowsutils"
)

func getMemoryInfo() (memoryInfo map[string]string, err error) {
	memoryInfo = make(map[string]string)
	computerSystem, err := utils.WindowsWMICommand("COMPUTERSYSTEM", "TotalPhysicalMemory")
	if err != nil {
		return
	}
	memoryInfo["total"] = computerSystem["TotalPhysicalMemory"]

	return
}
