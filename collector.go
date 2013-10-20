package verity

import (
	"github.com/kentaro/verity/cpu"
	"github.com/kentaro/verity/env"
	"github.com/kentaro/verity/hostname"
	"github.com/kentaro/verity/ipaddress"
	"github.com/kentaro/verity/ipv6address"
	"github.com/kentaro/verity/macaddress"
	"github.com/kentaro/verity/memory"
	"github.com/kentaro/verity/network"
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
