package verity

import (
	"github.com/kentaro/verity/cpu"
	"github.com/kentaro/verity/hostname"
	"github.com/kentaro/verity/memory"
	"log"
)

type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

var collectors = []Collector{
	&hostname.Hostname{},
	&cpu.Cpu{},
	&memory.Memory{},
}

func Collect() (result map[string]interface{}, err error) {
	result = make(map[string]interface{})

	for _, collector := range collectors {
		verity, err := collector.Collect()

		if err != nil {
			log.Printf("[%s] %s", collector.Name(), err)
			continue
		}

		result[collector.Name()] = verity
	}

	return
}
