package checks

import (
	"fmt"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// ProcessDiscovery is a ProcessDiscoveryCheck singleton. ProcessDiscoveryCheck should not be instantiated elsewhere.
var ProcessDiscovery = &ProcessDiscoveryCheck{probe: procutil.NewProcessProbe()}

// ProcessDiscoveryCheck is a check that gathers basic process metadata and sends it to the process discovery service.
// It uses its own ProcessDiscovery payload, which is intended to be read by the process_discovery kafka topic.
// The goal of this check is to collect information about possible integrations that may be enabled by the end user.
type ProcessDiscoveryCheck struct {
	probe      *procutil.Probe
	info       *model.SystemInfo
	initCalled bool
}

// Init initializes the ProcessDiscoveryCheck. It is a runtime error to call Run without first having called Init.
func (d *ProcessDiscoveryCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	d.info = info
	d.initCalled = true
}

// Name returns the name of the ProcessDiscoveryCheck. In this case it is "process_discovery"
func (d *ProcessDiscoveryCheck) Name() string { return config.DiscoveryCheckName }

// RealTime returns a value that says whether this check should be run in real time.
func (d *ProcessDiscoveryCheck) RealTime() bool { return false }

// Run collects process metadata, and packages it into a CollectorProcessDiscovery payload to be sent to process-intake.
// It is a runtime error to call Run without first having called Init.
func (d *ProcessDiscoveryCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	if !d.initCalled {
		return nil, fmt.Errorf("ProcessDiscoveryCheck.Run called before Init")
	}

	// Does not need to collect process stats, only metadata
	procs, err := getAllProcesses(d.probe, false)
	if err != nil {
		return nil, err
	}

	host := &model.Host{
		Name:        cfg.HostName,
		NumCpus:     d.info.Cpus[0].Number,
		TotalMemory: d.info.TotalMemory,
	}
	procDiscoveryChunks := chunkProcessDiscoveries(pidMaptoProcDiscoveryArray(procs, host), cfg.MaxPerMessage)
	payload := make([]model.MessageBody, len(procDiscoveryChunks))
	for i, procDiscoveryChunk := range procDiscoveryChunks {
		payload[i] = &model.CollectorProcDiscovery{
			HostName:           cfg.HostName,
			GroupId:            groupID,
			GroupSize:          int32(len(procDiscoveryChunks)),
			ProcessDiscoveries: procDiscoveryChunk,
			Host:               host,
		}
	}

	return payload, nil
}

func pidMaptoProcDiscoveryArray(pidMap map[int32]*procutil.Process, host *model.Host) []*model.ProcessDiscovery {
	array := make([]*model.ProcessDiscovery, 0, len(pidMap))
	for _, proc := range pidMap {
		array = append(array, &model.ProcessDiscovery{
			Pid:     proc.Pid,
			NsPid:   proc.NsPid,
			Host:    host,
			Command: formatCommand(proc),
			User:    formatUser(proc),
		})
	}

	return array
}

// chunkProcessDiscoveries split non-container processes into chunks and return a list of chunks
// This function is patiently awaiting go to support generics, so that we don't need two chunkProcesses functions :)
func chunkProcessDiscoveries(procs []*model.ProcessDiscovery, size int) [][]*model.ProcessDiscovery {
	chunkCount := len(procs) / size
	if chunkCount*size < len(procs) {
		chunkCount++
	}
	chunks := make([][]*model.ProcessDiscovery, 0, chunkCount)

	for i := 0; i < len(procs); i += size {
		end := i + size
		if end > len(procs) {
			end = len(procs)
		}
		chunks = append(chunks, procs[i:end])
	}

	return chunks
}
