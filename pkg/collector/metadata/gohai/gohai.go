package gohai

import (
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"

	log "github.com/cihub/seelog"
)

func GetPayload() *Payload {
	return &Payload{
		Gohai: getGohaiInfo(),
	}
}

func logError(module string, err error) {
	if err != nil {
		log.Errorf("Failed to retrieve %s metadata: %s", module, err)
	}
}

func getGohaiInfo() *gohai {

	cpuPayload, err := new(cpu.Cpu).Collect()
	logError("cpu", err)
	fileSystemPayload, err := new(filesystem.FileSystem).Collect()
	logError("filesystem", err)
	memoryPayload, err := new(memory.Memory).Collect()
	logError("memory", err)
	networkPayload, err := new(network.Network).Collect()
	logError("network", err)
	platformPayload, err := new(platform.Platform).Collect()
	logError("platform", err)

	return &gohai{
		Cpu:        cpuPayload,
		FileSystem: fileSystemPayload,
		Memory:     memoryPayload,
		Network:    networkPayload,
		Platform:   platformPayload,
	}
}
