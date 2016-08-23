package cpu

import (
	"fmt"
	utils "github.com/DataDog/gohai/windowsutils"
	"regexp"
	"strconv"
	"strings"
)

// Values that need to be multiplied by the number of physical processors
var perPhysicalProcValues = []string{
	"cpu_cores",
	"cpu_logical_processors",
}

func getCpuInfo() (cpuInfo map[string]string, err error) {

	cpuInfo = make(map[string]string)

	cpus, err := utils.WindowsWMIMultilineCommand("CPU",
		"CurrentClockSpeed", "Name", "NumberOfCores",
		"NumberOfLogicalProcessors", "Caption", "Manufacturer")
	if err != nil {
		return
	}
	// each line represents a different CPUx
	numberOfPhysicalCpus := len(cpus)
	cpu := cpus[0]

	cpuInfo["mhz"] = cpu["CurrentClockSpeed"]
	cpuInfo["model_name"] = cpu["Name"]
	cpuInfo["cpu_cores"] = cpu["NumberOfCores"]
	cpuInfo["cpu_logical_processors"] = cpu["NumberOfLogicalProcessors"]
	cpuInfo["vendor_id"] = cpu["Manufacturer"]

	caption := fmt.Sprintf(" %s ", cpu["Caption"])
	cpuInfo["family"] = extract(caption, "Family")
	cpuInfo["model"] = extract(caption, "Model")
	cpuInfo["stepping"] = extract(caption, "Stepping")

	// Multiply the values that are "per physical processor" by the number of physical procs
	for _, field := range perPhysicalProcValues {
		if value, ok := cpuInfo[field]; ok {
			intValue, err := strconv.Atoi(value)
			if err != nil {
				continue
			}

			cpuInfo[field] = strconv.Itoa(intValue * numberOfPhysicalCpus)
		}
	}

	return
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	return strings.Split(re.FindStringSubmatch(caption)[0], " ")[1]
}
