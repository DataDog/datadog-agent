package processes

import "flag"

var options struct {
	limit int
}

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
