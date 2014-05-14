package verity

import (
	"github.com/DataDog/verity/cpu"
	"github.com/DataDog/verity/filesystem"
	"github.com/DataDog/verity/memory"
	"github.com/DataDog/verity/network"
	"github.com/DataDog/verity/platform"
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
