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
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventType string

const (
	eventTypeStartService     = "start-service"
	eventTypeEndService       = "end-service"
	eventTypeHeartbeatService = "heartbeat-service"
)

type eventPayload struct {
	NamingSchemaVersion string `json:"naming_schema_version"`
	ServiceName         string `json:"service_name"`
	HostName            string `json:"host_name"`
	Env                 string `json:"env"`
	ServiceLanguage     string `json:"service_language"`
	ServiceType         string `json:"service_type"`
	StartTime           int64  `json:"start_time"`
	LastSeen            int64  `json:"last_seen"`
	APMInstrumentation  string `json:"apm_instrumentation"`
	ServiceNameSource   string `json:"service_name_source"`
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
	env := pkgconfig.Datadog.GetString("env")

	nameSource := "generated"
	if svc.meta.FromDDService {
		nameSource = "provided"
	}

	return &event{
		RequestType: t,
		APIVersion:  "v2",
		Payload: &eventPayload{
			NamingSchemaVersion: "1",
			ServiceName:         svc.meta.Name,
			HostName:            host,
			Env:                 env,
			ServiceLanguage:     svc.meta.Language,
			ServiceType:         svc.meta.Type,
			StartTime:           int64(svc.process.Stat.StartTime),
			LastSeen:            svc.LastHeartbeat.Unix(),
			APMInstrumentation:  svc.meta.APMInstrumentation,
			ServiceNameSource:   nameSource,
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
		svc.process.PID,
		svc.meta.Name,
		svc.process.Ports,
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
		svc.process.PID,
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
		svc.process.PID,
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
