// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"

	workloadmetacomp "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// languageMap is used to manually map from internal language type to the equivalent agent payload language type
// if a new language is added it must be added here as well, perhaps we can use an enum generator which can be used
// in tests to fail if not added here
var languageMap = map[languagemodels.LanguageName]model.Language{
	languagemodels.Unknown: model.Language_LANGUAGE_UNKNOWN,
	languagemodels.Go:      model.Language_LANGUAGE_GO,
	languagemodels.Node:    model.Language_LANGUAGE_NODE,
	languagemodels.Dotnet:  model.Language_LANGUAGE_DOTNET,
	languagemodels.Python:  model.Language_LANGUAGE_PYTHON,
	languagemodels.Java:    model.Language_LANGUAGE_JAVA,
	languagemodels.Ruby:    model.Language_LANGUAGE_RUBY,
	languagemodels.PHP:     model.Language_LANGUAGE_PHP,
	languagemodels.CPP:     model.Language_LANGUAGE_CPP,
}

// serviceNameSourceMap is used to manually map from internal service name source type to the equivalent agent payload service name source type
// if a new source is added it must be added here as well, perhaps we can use an enum generator which can be used
// in tests to fail if not added here
var serviceNameSourceMap = map[string]model.ServiceNameSource{
	"":                      model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
	string(usm.CommandLine): model.ServiceNameSource_SERVICE_NAME_SOURCE_COMMAND_LINE,
	string(usm.Laravel):     model.ServiceNameSource_SERVICE_NAME_SOURCE_LARAVEL,
	string(usm.Python):      model.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
	string(usm.Nodejs):      model.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
	string(usm.Gunicorn):    model.ServiceNameSource_SERVICE_NAME_SOURCE_GUNICORN,
	string(usm.Rails):       model.ServiceNameSource_SERVICE_NAME_SOURCE_RAILS,
	string(usm.Spring):      model.ServiceNameSource_SERVICE_NAME_SOURCE_SPRING,
	string(usm.JBoss):       model.ServiceNameSource_SERVICE_NAME_SOURCE_JBOSS,
	string(usm.Tomcat):      model.ServiceNameSource_SERVICE_NAME_SOURCE_TOMCAT,
	string(usm.WebLogic):    model.ServiceNameSource_SERVICE_NAME_SOURCE_WEBLOGIC,
	string(usm.WebSphere):   model.ServiceNameSource_SERVICE_NAME_SOURCE_WEBSPHERE,
}

// WLMProcessCollectionEnabled returns whether to use the workloadmeta process collector depending on the platform
// Currently, only enabled on linux.
func (p *ProcessCheck) WLMProcessCollectionEnabled() bool {
	return true
}

// processesByPID returns the processes by pid from workloadmeta.
func (p *ProcessCheck) processesByPID() (map[int32]*procutil.Process, error) {
	wlmProcList := p.wmeta.ListProcesses()
	pids := make([]int32, len(wlmProcList))
	for i, wlmProc := range wlmProcList {
		pids[i] = wlmProc.Pid
	}

	statsForProcess, err := p.probe.StatsForPIDs(pids, p.clock.Now())
	if err != nil {
		return nil, err
	}

	// map to common process type used by other versions of the check
	procs := make(map[int32]*procutil.Process, len(wlmProcList))
	for _, wlmProc := range wlmProcList {
		stats, exists := statsForProcess[wlmProc.Pid]
		// we need to check if the stats exist because there can be a lag between when a process is stored into WLM and when we query for its stats
		// we also want to verify that the stats are from the same collected process and not just the same PID coincidence by checking the start time
		// ex. a process is stopped but still exists in WLM, so the stats don't exist, and we shouldn't report it anymore,
		// additionally a new process with the same pid could spin up in between wlm collection and stat collection
		if !exists || (stats.CreateTime != wlmProc.CreationTime.UnixMilli()) {
			log.Debugf("stats do not exist for dead process %v - skipping", wlmProc.Pid)
			continue
		}
		procs[wlmProc.Pid] = mapWLMProcToProc(wlmProc, stats)
	}
	return procs, nil
}

// mapWLMProcToProc maps the workloadmeta process entity to an intermediate representation used in the process check
func mapWLMProcToProc(wlmProc *workloadmetacomp.Process, stats *procutil.Stats) *procutil.Process {
	var service *procutil.Service
	var tcpPorts, udpPorts []uint16
	portsCollected := false
	if wlmProc.Service != nil {
		service = &procutil.Service{
			GeneratedName:            wlmProc.Service.GeneratedName,
			GeneratedNameSource:      wlmProc.Service.GeneratedNameSource,
			AdditionalGeneratedNames: wlmProc.Service.AdditionalGeneratedNames,
			TracerMetadata:           wlmProc.Service.TracerMetadata,
			DDService:                wlmProc.Service.UST.Service,
			APMInstrumentation:       wlmProc.Service.APMInstrumentation,
			LogFiles:                 wlmProc.Service.LogFiles,
		}
		tcpPorts = wlmProc.Service.TCPPorts
		udpPorts = wlmProc.Service.UDPPorts
		portsCollected = true
	}
	return &procutil.Process{
		Pid:            wlmProc.Pid,
		Ppid:           wlmProc.Ppid,
		NsPid:          wlmProc.NsPid,
		Name:           wlmProc.Name,
		Cwd:            wlmProc.Cwd,
		Exe:            wlmProc.Exe,
		Comm:           wlmProc.Comm,
		Cmdline:        wlmProc.Cmdline,
		Uids:           wlmProc.Uids,
		Gids:           wlmProc.Gids,
		Stats:          stats,
		PortsCollected: portsCollected,
		TCPPorts:       tcpPorts,
		UDPPorts:       udpPorts,
		Language:       wlmProc.Language,
		Service:        service,
		InjectionState: procutil.InjectionState(wlmProc.InjectionState),
	}
}

// formatPorts converts separate TCP and UDP uint16 port lists to a int32 PortInfo
func formatPorts(portsCollected bool, tcpPorts, udpPorts []uint16) *model.PortInfo {
	// we want to semantically distinguish between the following cases:
	// - ports not being collected (by setting the PortInfo to nil)
	// - ports being collected, but no open ports for this process (a PortInfo with empty/nil ports)
	if !portsCollected {
		return nil
	}

	var newTCPPorts []int32
	if tcpPorts != nil {
		newTCPPorts = make([]int32, len(tcpPorts))
		for i, port := range tcpPorts {
			newTCPPorts[i] = int32(port)
		}
	}

	var newUDPPorts []int32
	if udpPorts != nil {
		newUDPPorts = make([]int32, len(udpPorts))
		for i, port := range udpPorts {
			newUDPPorts[i] = int32(port)
		}
	}

	return &model.PortInfo{
		Tcp: newTCPPorts,
		Udp: newUDPPorts,
	}
}

// formatLanguage converts a process language to the equivalent payload type
func formatLanguage(language *languagemodels.Language) model.Language {
	if language == nil {
		return model.Language_LANGUAGE_UNKNOWN
	}
	if lang, ok := languageMap[language.Name]; ok {
		return lang
	}
	return model.Language_LANGUAGE_UNKNOWN
}

// serviceNameSource maps a process's generated service name source to the equivalent agent payload type
func serviceNameSource(source string) model.ServiceNameSource {
	if modelSource, ok := serviceNameSourceMap[source]; ok {
		return modelSource
	}
	return model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN
}

// formatInjectionState converts the internal injection state to the agent payload enum
func formatInjectionState(state procutil.InjectionState) model.InjectionState {
	switch state {
	case procutil.InjectionInjected:
		return model.InjectionState_INJECTION_INJECTED
	case procutil.InjectionNotInjected:
		return model.InjectionState_INJECTION_NOT_INJECTED
	default:
		return model.InjectionState_INJECTION_UNKNOWN
	}
}

// formatServiceDiscovery converts collected service data into the equivalent agent payload type
func formatServiceDiscovery(service *procutil.Service) *model.ServiceDiscovery {
	if service == nil {
		return nil
	}
	source := serviceNameSource(service.GeneratedNameSource)

	var generatedServiceName *model.ServiceName
	if service.GeneratedName != "" {
		generatedServiceName = &model.ServiceName{
			Name:   service.GeneratedName,
			Source: source,
		}
	}

	var ddServiceName *model.ServiceName
	if service.DDService != "" {
		ddServiceName = &model.ServiceName{
			Name:   service.DDService,
			Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
		}
	}

	// additional generate names is not pre-allocated in order to potentially have a nil value instead of an empty slice
	var additionalGeneratedNames []*model.ServiceName
	for _, name := range service.AdditionalGeneratedNames {
		if name != "" {
			additionalGeneratedNames = append(additionalGeneratedNames, &model.ServiceName{
				Name:   name,
				Source: source,
			})
		}
	}

	// tracer metadata is not pre-allocated in order to potentially have a nil value instead of an empty slice
	var tracerMetadata []*model.TracerMetadata
	for _, tm := range service.TracerMetadata {
		tracerMetadata = append(tracerMetadata, &model.TracerMetadata{
			RuntimeId:   tm.RuntimeID,
			ServiceName: tm.ServiceName,
		})
	}

	var resources []*model.Resource
	for _, logPath := range service.LogFiles {
		resources = append(resources, &model.Resource{
			Resource: &model.Resource_Logs{
				Logs: &model.LogResource{
					Path: logPath,
				},
			},
		})
	}

	return &model.ServiceDiscovery{
		GeneratedServiceName:     generatedServiceName,
		DdServiceName:            ddServiceName,
		AdditionalGeneratedNames: additionalGeneratedNames,
		TracerMetadata:           tracerMetadata,
		ApmInstrumentation:       service.APMInstrumentation,
		Resources:                resources,
	}
}
