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
	NamingSchemaVersion  string   `json:"naming_schema_version"`
	ServiceName          string   `json:"service_name"`
	GeneratedServiceName string   `json:"generated_service_name"`
	DDService            string   `json:"dd_service,omitempty"`
	HostName             string   `json:"host_name"`
	Env                  string   `json:"env"`
	ServiceLanguage      string   `json:"service_language"`
	ServiceType          string   `json:"service_type"`
	StartTime            int64    `json:"start_time"`
	StartTimeMilli       int64    `json:"start_time_milli"`
	LastSeen             int64    `json:"last_seen"`
	APMInstrumentation   string   `json:"apm_instrumentation"`
	ServiceNameSource    string   `json:"service_name_source,omitempty"`
	Ports                []uint16 `json:"ports"`
	PID                  int      `json:"pid"`
	CommandLine          []string `json:"command_line"`
	RSSMemory            uint64   `json:"rss_memory"`
	CPUCores             float64  `json:"cpu_cores"`
	ContainerID          string   `json:"container_id"`
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

func (ts *telemetrySender) newEvent(t eventType, svc serviceInfo) *event {
	host := ts.hostname.GetSafe(context.Background())
	env := pkgconfigsetup.Datadog().GetString("env")

	nameSource := ""
	if svc.service.DDService != "" {
		nameSource = "provided"
		if svc.service.DDServiceInjected {
			nameSource = "injected"
		}
	}

	return &event{
		RequestType: t,
		APIVersion:  "v2",
		Payload: &eventPayload{
			NamingSchemaVersion:  "1",
			ServiceName:          svc.meta.Name,
			GeneratedServiceName: svc.service.GeneratedName,
			DDService:            svc.service.DDService,
			HostName:             host,
			Env:                  env,
			ServiceLanguage:      svc.meta.Language,
			ServiceType:          svc.meta.Type,
			StartTime:            int64(svc.service.StartTimeMilli / 1000),
			StartTimeMilli:       int64(svc.service.StartTimeMilli),
			LastSeen:             svc.LastHeartbeat.Unix(),
			APMInstrumentation:   svc.meta.APMInstrumentation,
			ServiceNameSource:    nameSource,
			Ports:                svc.service.Ports,
			PID:                  svc.service.PID,
			CommandLine:          svc.service.CommandLine,
			RSSMemory:            svc.service.RSS,
			CPUCores:             svc.service.CPUCores,
			ContainerID:          svc.service.ContainerID,
		},
	}
}

func newTelemetrySender(sender sender.Sender) *telemetrySender {
	return &telemetrySender{
		sender:   sender,
		hostname: hostnameimpl.NewHostnameService(),
	}
}

func (ts *telemetrySender) sendStartServiceEvent(svc serviceInfo) {
	log.Debugf("[pid: %d | name: %s | ports: %v] start-service",
		svc.service.PID,
		svc.meta.Name,
		svc.service.Ports,
	)

	e := ts.newEvent(eventTypeStartService, svc)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode start-service event as json: %v", err)
		return
	}
	log.Debugf("sending start-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendHeartbeatServiceEvent(svc serviceInfo) {
	log.Debugf("[pid: %d | name: %s] heartbeat-service",
		svc.service.PID,
		svc.meta.Name,
	)

	e := ts.newEvent(eventTypeHeartbeatService, svc)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode heartbeat-service event as json: %v", err)
		return
	}
	log.Debugf("sending heartbeat-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendEndServiceEvent(svc serviceInfo) {
	log.Debugf("[pid: %d | name: %s] end-service",
		svc.service.PID,
		svc.meta.Name,
	)

	e := ts.newEvent(eventTypeEndService, svc)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode end-service event as json: %v", err)
		return
	}
	log.Debugf("sending end-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}
