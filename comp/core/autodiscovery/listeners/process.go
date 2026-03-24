// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessListener listens to process creation through a subscription to the
// workloadmeta store. It only considers processes that have been identified
// as services (i.e., processes with a non-nil Service property).
type ProcessListener struct {
	workloadmetaListener
	tagger         tagger.Component
	processFilters workloadfilter.FilterBundle
}

// NewProcessListener returns a new ProcessListener.
func NewProcessListener(options ServiceListernerDeps) (ServiceListener, error) {
	const name = "ad-processlistener"
	l := &ProcessListener{
		tagger:         options.Tagger,
		processFilters: options.Filter.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELGlobal}}),
	}
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceAll).
		AddKind(workloadmeta.KindProcess).Build()

	wmetaInstance, ok := options.Wmeta.Get()
	if !ok {
		return nil, errors.New("workloadmeta store is not initialized")
	}
	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, filter, l.createProcessService, wmetaInstance, options.Telemetry)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func isMainProcessForService(process *workloadmeta.Process, wmeta workloadmeta.Component) bool {
	// If no parent or parent is the init process, then this process is the main
	// process.
	if process.Ppid == 0 || process.Ppid == 1 {
		return true
	}

	parent, err := wmeta.GetProcess(process.Ppid)
	if err != nil {
		// The parent doesn't exist in WLM, so we assume that we are the main
		// process. Note that if the parent and the child process have been
		// collected at the same time, the event for the child process could be
		// received before the event for the parent process. However, since WLM
		// calls the events after creating all the entities in WLM (see
		// store.go:handleEvents()), the GetProcess call should return the
		// parent process in that case too.
		return true
	}

	// If the parent process has no service data, then we assume that we are the
	// main process.
	if parent.Service == nil {
		return true
	}

	// If the parent has the same GeneratedName, then we assume that we are not
	// the main process (the parent is). We use GeneratedName rather than Comm
	// to avoid false matches between unrelated services that share the same
	// interpreter (eg supervisord and flask both have Comm="python").
	return parent.Service.GeneratedName != process.Service.GeneratedName
}

func (l *ProcessListener) createProcessService(entity workloadmeta.Entity) {
	process := entity.(*workloadmeta.Process)

	// Only consider processes that have been identified as services
	// (i.e., processes with a non-nil Service property)
	if process.Service == nil {
		log.Tracef("process %d (%s) has no service data, skipping", process.Pid, process.Comm)
		return
	}

	// Skip container-bound processes to avoid duplicate checks
	// Container-bound processes are already handled by the container listener
	if process.ContainerID != "" {
		log.Debugf("process %d (%s) is container-bound (container: %s), skipping", process.Pid, process.Comm, process.ContainerID)
		return
	}

	if !isMainProcessForService(process, l.Store()) {
		log.Tracef("process %d (%s) is not the main process of the service, skipping", process.Pid, process.Comm)
		return
	}

	if l.processFilters != nil {
		filterableProcess := workloadmetafilter.CreateProcess(process)
		if l.processFilters.IsExcluded(filterableProcess) {
			log.Debugf("Process %d excluded from AD process listener by CEL filter", process.Pid)
			return
		}
	}

	// Build ports from Service.TCPPorts
	ports := make([]workloadmeta.ContainerPort, 0, len(process.Service.TCPPorts))
	for _, port := range process.Service.TCPPorts {
		ports = append(ports, workloadmeta.ContainerPort{
			Port: int(port),
		})
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	svc := &ProcessService{
		process:  process,
		tagsHash: l.tagger.GetEntityHash(types.NewEntityID(types.Process, process.EntityID.ID), types.ChecksConfigCardinality),
		ports:    ports,
		pid:      int(process.Pid),
		// Host processes are accessible at localhost
		hosts:  map[string]string{"host": "127.0.0.1"},
		ready:  true,
		tagger: l.tagger,
		wmeta:  l.Store(),
	}

	svcID := buildSvcID(process.GetID())
	l.AddService(svcID, svc, "")
}

// ProcessService implements the Service interface for process entities.
type ProcessService struct {
	process  *workloadmeta.Process
	tagsHash string
	hosts    map[string]string
	ports    []workloadmeta.ContainerPort
	pid      int
	ready    bool
	tagger   tagger.Component
	wmeta    workloadmeta.Component
}

var _ Service = &ProcessService{}

// Equal returns whether the two services are equal
func (s *ProcessService) Equal(o Service) bool {
	s2, ok := o.(*ProcessService)
	if !ok {
		return false
	}

	return s.GetServiceID() == s2.GetServiceID() &&
		s.tagsHash == s2.tagsHash &&
		reflect.DeepEqual(s.hosts, s2.hosts) &&
		reflect.DeepEqual(s.ports, s2.ports) &&
		s.pid == s2.pid &&
		s.ready == s2.ready
}

// GetServiceID returns the AD entity ID of the service.
func (s *ProcessService) GetServiceID() string {
	return "process://" + s.process.EntityID.ID
}

// GetADIdentifiers returns the service's AD identifiers.
func (s *ProcessService) GetADIdentifiers() []string {
	return []string{string(adtypes.CelProcessIdentifier)}
}

// GetHosts returns the service's IPs for each host.
func (s *ProcessService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPorts returns the ports exposed by the service.
func (s *ProcessService) GetPorts() ([]workloadmeta.ContainerPort, error) {
	return s.ports, nil
}

// GetTags returns the tags associated with the service.
func (s *ProcessService) GetTags() ([]string, error) {
	return s.tagger.Tag(taggercommon.BuildTaggerEntityID(s.process.GetID()), types.ChecksConfigCardinality)
}

// GetTagsWithCardinality returns the tags with given cardinality.
func (s *ProcessService) GetTagsWithCardinality(cardinality string) ([]string, error) {
	checkCard, err := types.StringToTagCardinality(cardinality)
	if err == nil {
		return s.tagger.Tag(taggercommon.BuildTaggerEntityID(s.process.GetID()), checkCard)
	}
	log.Warnf("error converting cardinality %s to TagCardinality: %v", cardinality, err)
	return s.GetTags()
}

// GetPid returns the process ID of the service.
func (s *ProcessService) GetPid() (int, error) {
	return s.pid, nil
}

// GetHostname returns the service's hostname.
func (s *ProcessService) GetHostname() (string, error) {
	return "", nil
}

// IsReady returns whether the service is ready.
func (s *ProcessService) IsReady() bool {
	return s.ready
}

// HasFilter returns whether the service should not collect certain data (logs
// or metrics) due to filtering applied by filter.
func (s *ProcessService) HasFilter(_ workloadfilter.Scope) bool {
	// Process services don't have container-style filtering
	return false
}

// FilterTemplates implements Service#FilterTemplates.
func (s *ProcessService) FilterTemplates(configs map[string]integration.Config) {
	filterTemplatesMatched(s, configs)
}

// GetExtraConfig returns extra configuration associated with the service.
func (s *ProcessService) GetExtraConfig(key string) (string, error) {
	return "", fmt.Errorf("extra config %q is not supported for process services", key)
}

// GetImageName returns the image name for the monitored entity.
// Not applicable for processes.
func (s *ProcessService) GetImageName() string {
	return ""
}

// GetFilterableEntity returns the filterable entity of the service
func (s *ProcessService) GetFilterableEntity() workloadfilter.Filterable {
	return workloadmetafilter.CreateProcess(s.process)
}
