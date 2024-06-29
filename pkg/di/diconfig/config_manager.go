package diconfig

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/codegen"
	"github.com/DataDog/datadog-agent/pkg/di/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/di/ebpf"
	"github.com/DataDog/datadog-agent/pkg/di/eventparser"
	"github.com/DataDog/datadog-agent/pkg/di/proctracker"
	"github.com/DataDog/datadog-agent/pkg/di/ratelimiter"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/google/uuid"
)

type rcConfig struct {
	ID        string
	Version   int
	ProbeType string `json:"type"`
	Language  string
	Where     struct {
		TypeName   string `json:"typeName"`
		MethodName string `json:"methodName"`
		SourceFile string
		Lines      []string
	}
	Tags            []string
	Template        string
	CaptureSnapshot bool
	EvaluatedAt     string
}

type configUpdateCallback func(*ditypes.ProcessInfo, *ditypes.Probe)

type ConfigManager interface {
	GetProcInfos() ditypes.DIProcs
	Stop()
}

type RCConfigManager struct {
	procTracker *proctracker.ProcessTracker

	diProcs  ditypes.DIProcs
	callback configUpdateCallback
}

func NewRCConfigManager() (*RCConfigManager, error) {
	log.Info("Creating new config manager")
	cm := &RCConfigManager{
		callback: applyConfigUpdate,
	}

	cm.procTracker = proctracker.NewProcessTracker(cm.updateProcesses)
	err := cm.procTracker.Start()
	if err != nil {
		return nil, fmt.Errorf("could not start proccess tracker: %w", err)
	}
	cm.diProcs = ditypes.NewDIProcs()
	return cm, nil
}

func (cm *RCConfigManager) GetProcInfos() ditypes.DIProcs {
	return cm.diProcs
}

func (cm *RCConfigManager) Stop() {
	cm.procTracker.Stop()
	for _, procInfo := range cm.GetProcInfos() {
		procInfo.CloseAllUprobeLinks()
	}
}

// updateProcesses is the callback interface that ConfigManager uses to consume the map of `ProcessInfo`s
// It is called whenever there's an update to the state of known processes of services on the machine.
//
// It compares the previously known state of services on the machine and creates a hook on the remote-config
// callback for configurations on new ones, and deletes the hook on old ones.
func (cm *RCConfigManager) updateProcesses(runningProcs ditypes.DIProcs) {
	// Remove processes that are no longer running from state and close their uprobe links
	for pid, procInfo := range cm.diProcs {
		_, ok := runningProcs[pid]
		if !ok {
			procInfo.CloseAllUprobeLinks()
			delete(cm.diProcs, pid)
		}
	}

	for pid, runningProcInfo := range runningProcs {
		_, ok := cm.diProcs[pid]
		if !ok {
			cm.diProcs[pid] = runningProcInfo
			err := cm.installConfigProbe(runningProcInfo)
			if err != nil {
				log.Infof("could not install config probe for service %s (pid %d): %s", runningProcInfo.ServiceName, runningProcInfo.PID, err)
			}
		}
	}
}

func (cm *RCConfigManager) installConfigProbe(procInfo *ditypes.ProcessInfo) error {
	var err error
	configProbe := newConfigProbe()

	svcConfigProbe := *configProbe
	svcConfigProbe.ServiceName = procInfo.ServiceName
	procInfo.ProbesByID[configProbe.ID] = &svcConfigProbe

	err = analyzeBinary(procInfo)
	if err != nil {
		return fmt.Errorf("could not analyze binary for config probe: %w", err)
	}

	err = codegen.GenerateBPFProgram(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not generate bpf code for config probe: %w", err)
	}

	err = ebpf.CompileBPFProgram(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not compile bpf code for config probe: %w", err)
	}

	err = ebpf.AttachBPFUprobe(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not attach bpf code for config probe: %w", err)
	}

	m, err := procInfo.SetupConfigUprobe()
	if err != nil {
		return fmt.Errorf("could not setup config probe for service %s: %w", procInfo.ServiceName, err)
	}

	r, err := ringbuf.NewReader(m)
	if err != nil {
		return fmt.Errorf("could not read from config probe %s", procInfo.ServiceName)
	}

	go cm.readConfigs(r, procInfo)

	return nil
}

func (cm *RCConfigManager) readConfigs(r *ringbuf.Reader, procInfo *ditypes.ProcessInfo) {
	for {
		log.Info("Waiting for configs")
		record, err := r.Read()
		if err != nil {
			log.Infof("error reading raw configuration from bpf: %s", err)
			continue
		}

		configEventParams, err := eventparser.ParseParams(record.RawSample)
		if err != nil {
			log.Infof("error parsing configuration for PID %d: %s", procInfo.PID, err)
			continue
		}
		if len(configEventParams) != 3 {
			log.Infof("error parsing configuration for PID %d: not enough arguments", procInfo.PID)
			continue
		}

		runtimeID, err := uuid.ParseBytes([]byte(configEventParams[0].ValueStr))
		if err != nil {
			log.Infof("Runtime ID \"%s\" is not a UUID: %s)\n", runtimeID, err)
			continue
		}

		configPath, err := ditypes.ParseConfigPath(string(configEventParams[1].ValueStr))
		if err != nil {
			log.Infof("couldn't parse config path: %v", err)
			continue
		}

		// An empty config means that this probe has been removed for this process
		if configEventParams[2].ValueStr == "" {
			cm.diProcs.DeleteProbe(procInfo.PID, configPath.ProbeUUID.String())
			continue
		}

		conf := rcConfig{}
		err = json.Unmarshal([]byte(configEventParams[2].ValueStr), &conf)
		if err != nil {
			diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, configPath.ProbeUUID.String(), "ATTACH_ERROR", err.Error())
			log.Infof("could not unmarshal configuration, cannot apply: %s (Probe-ID: %s)\n", err, configPath.ProbeUUID)
			continue
		}

		opts := &ditypes.InstrumentationOptions{
			ArgumentsMaxSize: ditypes.ArgumentsMaxSize,
			StringMaxSize:    ditypes.StringMaxSize,
		}

		probe, probeExists := procInfo.ProbesByID[configPath.ProbeUUID.String()]
		if !probeExists {
			cm.diProcs.SetProbe(procInfo.PID, procInfo.ServiceName, conf.Where.TypeName, conf.Where.MethodName, configPath.ProbeUUID, runtimeID, opts)
			diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, runtimeID.String(), configPath.ProbeUUID.String(), ditypes.StatusReceived)
			probe = procInfo.ProbesByID[configPath.ProbeUUID.String()]
		}

		// Check hash to see if the configuration changed
		if configPath.Hash != probe.InstrumentationInfo.ConfigurationHash {
			probe.InstrumentationInfo.ConfigurationHash = configPath.Hash
			applyConfigUpdate(procInfo, probe)
		}
	}
}

func applyConfigUpdate(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) {
	log.Info("Applying config update", probe)
	err := analyzeBinary(procInfo)
	if err != nil {
		log.Infof("couldn't inspect binary: %s\n", err)
		return
	}

	err = codegen.GenerateBPFProgram(procInfo, probe)
	if err != nil {
		log.Info("Couldn't generate BPF programs", err)
		return
	}

	err = ebpf.CompileBPFProgram(procInfo, probe)
	if err != nil {
		log.Info("Couldn't compile BPF objects", err)
		return
	}

	err = ebpf.AttachBPFUprobe(procInfo, probe)
	if err != nil {
		log.Info("Errors while attaching bpf programs", err)
		return
	}
}

func newConfigProbe() *ditypes.Probe {
	return &ditypes.Probe{
		ID:       ditypes.ConfigBPFProbeID,
		FuncName: "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.passProbeConfiguration",
		InstrumentationInfo: &ditypes.InstrumentationInfo{
			InstrumentationOptions: &ditypes.InstrumentationOptions{
				ArgumentsMaxSize: 100000,
				StringMaxSize:    30000,
			},
		},
		RateLimiter: ratelimiter.NewSingleEventRateLimiter(0),
	}
}
