// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventType string

const (
	eventTypeStartService     = "start-service"
	eventTypeEndService       = "end-service"
	eventTypeHeartbeatService = "heartbeat-service"
)

type eventPayload struct {
	NamingSchemaVersion        string   `json:"naming_schema_version"`
	ServiceName                string   `json:"service_name"`
	GeneratedServiceName       string   `json:"generated_service_name"`
	GeneratedServiceNameSource string   `json:"generated_service_name_source,omitempty"`
	AdditionalGeneratedNames   []string `json:"additional_generated_names,omitempty"`
	ContainerServiceName       string   `json:"container_service_name,omitempty"`
	ContainerServiceNameSource string   `json:"container_service_name_source,omitempty"`
	ContainerTags              []string `json:"container_tags,omitempty"`
	DDService                  string   `json:"dd_service,omitempty"`
	HostName                   string   `json:"host_name"`
	Env                        string   `json:"env"`
	ServiceLanguage            string   `json:"service_language"`
	ServiceType                string   `json:"service_type"`
	StartTime                  int64    `json:"start_time"`
	StartTimeMilli             int64    `json:"start_time_milli"`
	LastSeen                   int64    `json:"last_seen"`
	APMInstrumentation         string   `json:"apm_instrumentation"`
	ServiceNameSource          string   `json:"service_name_source,omitempty"`
	Ports                      []uint16 `json:"ports"`
	PID                        int      `json:"pid"`
	CommandLine                []string `json:"command_line"`
	RSSMemory                  uint64   `json:"rss_memory"`
	CPUCores                   float64  `json:"cpu_cores"`
	ContainerID                string   `json:"container_id"`
	RxBytes                    uint64   `json:"rx_bytes"`
	TxBytes                    uint64   `json:"tx_bytes"`
	RxBps                      float64  `json:"rx_bps"`
	TxBps                      float64  `json:"tx_bps"`
}

type event struct {
	RequestType eventType     `json:"request_type"`
	APIVersion  string        `json:"api_version"`
	Payload     *eventPayload `json:"payload"`
}

type telemetrySender struct {
	sender   sender.Sender
	hostname hostname.Component
}

func (ts *telemetrySender) newEvent(t eventType, service model.Service) *event {
	host := ts.hostname.GetSafe(context.Background())
	env := pkgconfigsetup.Datadog().GetString("env")

	nameSource := ""
	if service.DDService != "" {
		nameSource = "provided"
		if service.DDServiceInjected {
			nameSource = "injected"
		}
	}

	return &event{
		RequestType: t,
		APIVersion:  "v2",
		Payload: &eventPayload{
			NamingSchemaVersion:        "1",
			ServiceName:                service.Name,
			GeneratedServiceName:       service.GeneratedName,
			GeneratedServiceNameSource: service.GeneratedNameSource,
			AdditionalGeneratedNames:   service.AdditionalGeneratedNames,
			ContainerServiceName:       service.ContainerServiceName,
			ContainerServiceNameSource: service.ContainerServiceNameSource,
			ContainerTags:              service.ContainerTags,
			DDService:                  service.DDService,
			HostName:                   host,
			Env:                        env,
			ServiceLanguage:            service.Language,
			ServiceType:                service.Type,
			StartTime:                  int64(service.StartTimeMilli / 1000),
			StartTimeMilli:             int64(service.StartTimeMilli),
			LastSeen:                   service.LastHeartbeat,
			APMInstrumentation:         service.APMInstrumentation,
			ServiceNameSource:          nameSource,
			Ports:                      service.Ports,
			PID:                        service.PID,
			CommandLine:                service.CommandLine,
			RSSMemory:                  service.RSS,
			CPUCores:                   service.CPUCores,
			ContainerID:                service.ContainerID,
			RxBytes:                    service.RxBytes,
			TxBytes:                    service.TxBytes,
			RxBps:                      service.RxBps,
			TxBps:                      service.TxBps,
		},
	}
}

func newTelemetrySender(sender sender.Sender) *telemetrySender {
	return &telemetrySender{
		sender:   sender,
		hostname: hostnameimpl.NewHostnameService(),
	}
}

func (ts *telemetrySender) sendStartServiceEvent(service model.Service) {
	log.Debugf("[pid: %d | name: %s | ports: %v] start-service",
		service.PID,
		service.Name,
		service.Ports,
	)

	e := ts.newEvent(eventTypeStartService, service)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode start-service event as json: %v", err)
		return
	}
	log.Debugf("sending start-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendHeartbeatServiceEvent(service model.Service) {
	log.Debugf("[pid: %d | name: %s] heartbeat-service",
		service.PID,
		service.Name,
	)

	e := ts.newEvent(eventTypeHeartbeatService, service)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode heartbeat-service event as json: %v", err)
		return
	}
	log.Debugf("sending heartbeat-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendEndServiceEvent(service model.Service) {
	log.Debugf("[pid: %d | name: %s] end-service",
		service.PID,
		service.Name,
	)

	e := ts.newEvent(eventTypeEndService, service)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode end-service event as json: %v", err)
		return
	}
	log.Debugf("sending end-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}
