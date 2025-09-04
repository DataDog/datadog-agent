// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"errors"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessListener listens to process creation through a subscription to the
// workloadmeta store.
type ProcessListener struct {
	workloadmetaListener
	filterStore filter.Component
	tagger      tagger.Component
	containerProvider proccontainers.ContainerProvider
}

// NewProcessListener returns a new ProcessListener.
func NewProcessListener(options ServiceListernerDeps) (ServiceListener, error) {
	log.Debugf("[Checkpoint] NewProcessListener started.")
	const name = "ad-processlistener"

	containerProvider, err := proccontainers.GetSharedContainerProvider()
	if err != nil {
		return nil, err
	}
	
	l := &ProcessListener{
		filterStore: options.Filter,
		tagger:      options.Tagger,
		containerProvider: containerProvider,
	}

	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceAll).
		AddKindWithEntityFilter(
			workloadmeta.KindProcess,
			func(e workloadmeta.Entity) bool {
				p := e.(*workloadmeta.Process)
				return p.Service != nil
			}).Build()

	log.Debugf("[Checkpoint] filter built.")

	wmetaInstance, ok := options.Wmeta.Get()
	if !ok {
		log.Debugf("[Checkpoint] Failed to get workloadmeta instance.")
		return nil, errors.New("workloadmeta store is not initialized")
	}
	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, filter, l.createProcessService, wmetaInstance, options.Telemetry)
	if err != nil {
		log.Debugf("[Checkpoint] faailed to create workloadmeta listener.")
		return nil, err
	}

	return l, nil
}

func (l *ProcessListener) createProcessService(entity workloadmeta.Entity) {
	process := entity.(*workloadmeta.Process)

	comm := process.Comm
	p_service := process.Service

	p_ports := p_service.Ports

	ports := make([]ContainerPort, 0, len(p_ports))
	for _, p := range p_ports {
		ports = append(ports, ContainerPort{
			Port: int(p),
		})
	}

	log.Debugf("Received service entity %q with comm %q and cmdline %v", process.GetID(), comm, process.Cmdline)
	if comm == "redis" || comm == "redis-server" {
		log.Debugf("Scheduling redis check.")
		svc := &service{
			entity:        process,
			tagsHash:      l.tagger.GetEntityHash(types.NewEntityID(types.Process, process.ID), types.ChecksConfigCardinality),
			adIdentifiers: []string{"redis"},
			ports:         ports,
			pid:           int(process.Pid),
			tagger:        l.tagger,
			ready:         true,
			checkNames:    []string{"redis"},
			hosts: map[string]string{
				"localhost": "127.0.0.1",
			},
		}

		svcID := buildSvcID(process.GetID())
		l.AddService(svcID, svc, "")
	}

}
