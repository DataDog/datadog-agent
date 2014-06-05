package cpu

import (
	utils "github.com/DataDog/gohai/windowsutils"
	// "strconv"
	"fmt"
	"regexp"
	"strings"
)

var cpuMap = map[string]string{
	"machdep.cpu.vendor":       "vendor_id",
	"machdep.cpu.brand_string": "model_name",
	"hw.physicalcpu":           "cpu_cores",
	"hw.cpufrequency":          "mhz",
	"machdep.cpu.family":       "family",
	"machdep.cpu.model":        "model",
	"machdep.cpu.stepping":     "stepping",
}

func getCpuInfo() (cpuInfo map[string]string, err error) {

	cpuInfo = make(map[string]string)

	cpu, err := utils.WindowsWMICommand("CPU",
		"CurrentClockSpeed", "Name", "NumberOfCores",
		"Caption", "Manufacturer")
	if err != nil {
		return
	}
	cpuInfo["mhz"] = cpu["CurrentClockSpeed"]
	cpuInfo["model_name"] = cpu["Name"]
	cpuInfo["cpu_cores"] = cpu["NumberOfCores"]
	cpuInfo["vendor_id"] = cpu["Manufacturer"]

	caption := fmt.Sprintf(" %s ", cpu["Caption"])
	cpuInfo["family"] = extract(caption, "Family")
	cpuInfo["model"] = extract(caption, "Model")
	cpuInfo["stepping"] = extract(caption, "Stepping")

	return
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	return strings.Split(re.FindStringSubmatch(caption)[0], " ")[1]
}
