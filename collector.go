package verity

import (
	"github.com/Datadog/verity/cpu"
	"github.com/Datadog/verity/filesystem"
	"github.com/Datadog/verity/memory"
	"github.com/Datadog/verity/network"
	"github.com/Datadog/verity/platform"
	"log"
)

type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

var collectors = []Collector{
	&cpu.Cpu{},
	&filesystem.FileSystem{},
	&memory.Memory{},
	&network.Network{},
	&platform.Platform{},
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
