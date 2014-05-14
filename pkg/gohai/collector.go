package verity

import (
	"github.com/Datadog/verity/cpu"
	"github.com/Datadog/verity/hostname"
	"github.com/Datadog/verity/filesystem"
	"github.com/Datadog/verity/ipaddress"
	"github.com/Datadog/verity/ipv6address"
	"github.com/Datadog/verity/macaddress"
	"github.com/Datadog/verity/memory"
	"log"
)

type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

var collectors = []Collector{
	&cpu.Cpu{},
	&hostname.Hostname{},
	&filesystem.FileSystem{},
	&ipaddress.IpAddress{},
	&ipv6address.Ipv6Address{},
	&macaddress.MacAddress{},
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
