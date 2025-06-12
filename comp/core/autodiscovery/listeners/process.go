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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessListener listens to process creation through a subscription to the
// workloadmeta store.
type ProcessListener struct {
	workloadmetaListener
	tagger tagger.Component
}

// NewProcessListener returns a new ProcessListener.
func NewProcessListener(options ServiceListernerDeps) (ServiceListener, error) {
	const name = "ad-processlistener"
	l := &ProcessListener{}
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceServiceDiscovery).
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
	l.tagger = options.Tagger

	return l, nil
}

func (l *ProcessListener) createProcessService(entity workloadmeta.Entity) {
	process := entity.(*workloadmeta.Process)
	if process.Service == nil {
		return
	}

	log.Debugf("Creating process service: %#v", process)
	log.Debugf("Creating process service: %#v", process.Service)

	// Check if we have any service names
	hasServiceName := process.Service.GeneratedName != "" ||
		process.Service.ContainerServiceName != "" ||
		process.Service.DDService != "" ||
		len(process.Service.AdditionalGeneratedNames) > 0

	if !hasServiceName {
		return
	}

	// Create service identifiers
	adIdentifiers := []string{process.Service.GeneratedName}
	if process.Service.ContainerServiceName != "" {
		adIdentifiers = append(adIdentifiers, process.Service.ContainerServiceName)
	}
	if process.Service.DDService != "" {
		adIdentifiers = append(adIdentifiers, process.Service.DDService)
	}
	adIdentifiers = append(adIdentifiers, process.Service.AdditionalGeneratedNames...)

	// Create ports
	ports := make([]ContainerPort, 0, len(process.Service.Ports))
	for _, port := range process.Service.Ports {
		ports = append(ports, ContainerPort{
			Port: int(port),
		})
	}

	svc := &service{
		entity:        process,
		tagsHash:      l.tagger.GetEntityHash(types.NewEntityID(types.Process, process.GetID().ID), types.ChecksConfigCardinality),
		adIdentifiers: adIdentifiers,
		ports:         ports,
		pid:           int(process.Pid),
		hostname:      process.Service.GeneratedName,
		tagger:        l.tagger,
		hosts:         map[string]string{"host": "127.0.0.1"},
		ready:         true,
	}

	// Add container tags if available
	// (Remove the line: svc.tags = process.Service.ContainerTags)

	svcID := buildSvcID(process.GetID())
	l.AddService(svcID, svc, "")
}
