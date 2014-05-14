package verity

import (
	"github.com/Datadog/verity/cpu"
	"github.com/Datadog/verity/env"
	"github.com/Datadog/verity/hostname"
	"github.com/Datadog/verity/ipaddress"
	"github.com/Datadog/verity/ipv6address"
	"github.com/Datadog/verity/macaddress"
	"github.com/Datadog/verity/memory"
	"github.com/Datadog/verity/network"
	"log"
)

type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

var collectors = []Collector{
	&cpu.Cpu{},
	&env.Env{},
	&hostname.Hostname{},
	&ipaddress.IpAddress{},
	&ipv6address.Ipv6Address{},
	&macaddress.MacAddress{},
	&memory.Memory{},
	&network.Network{},
}

func Collect() (result map[string]interface{}, err error) {
	result = make(map[string]interface{})

	for _, collector := range collectors {
		verity, err := collector.Collect()

		if err != nil {
			log.Printf("[%s] %s", collector.Name(), err)
			continue
		}

		// We put the values from environment variables on to the top level of the result.
		if collector.Name() == "env" {
			for key, value := range verity.(map[string]string) {
				result[key] = value
			}
		} else {
			result[collector.Name()] = verity
		}
	}

	return
}
