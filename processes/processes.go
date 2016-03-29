package processes

import (
	"flag"
	"strings"
	"time"

	"github.com/DataDog/gohai/processes/gops"
)

var options struct {
	limit int
}

type ProcessField [7]interface{}

type Processes struct{}

const name = "processes"

func init() {
	flag.IntVar(&options.limit, name+"-limit", 20, "Number of process groups to return")
}

func (self *Processes) Name() string {
	return name
}

func (self *Processes) Collect() (result interface{}, err error) {
	result, err = getProcesses(options.limit)
	return
}

// Return a JSON payload that's compatible with the legacy "processes" resource check
func getProcesses(limit int) ([]interface{}, error) {
	processGroups, err := gops.TopRSSProcessGroups(limit)
	if err != nil {
		return nil, err
	}

	snapData := make([]ProcessField, len(processGroups))

	for i, processGroup := range processGroups {
		processField := ProcessField{
			strings.Join(processGroup.Usernames(), ","),
			0, // pct_cpu, requires two consecutive samples to be computed, so not fetched for now
			processGroup.PctMem(),
			processGroup.VMS(),
			processGroup.RSS(),
			processGroup.Name(),
			len(processGroup.Pids()),
		}
		snapData[i] = processField
	}

	return []interface{}{time.Now().Unix(), snapData}, nil
}
