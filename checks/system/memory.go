package system

import
// stdlib
// project

// 3rd party

(
	"fmt"

	"github.com/DataDog/datadog-agent/checks"
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/mem"
)

var log = logging.MustGetLogger("datadog-agent")

type MemoryCheck struct{}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

func (c *MemoryCheck) Run() (checks.CheckResult, error) {
	v, _ := mem.VirtualMemory()
	res := fmt.Sprintf(`{"gauge": [{"Name": "system.mem.total", "Value": %f, "Tags": null}]}`, float64(v.Total))
	checkRes := checks.CheckResult{Result: res, Error: ""}
	return checkRes, nil
}
