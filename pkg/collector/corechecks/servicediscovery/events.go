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
	ApiVersion          string    `json:"api_version"`
	NamingSchemaVersion string    `json:"naming_schema_version"`
	RequestType         eventType `json:"request_type"`
	ServiceName         string    `json:"service_name"`
	HostName            string    `json:"host_name"`
	Env                 string    `json:"env"`
	ServiceLanguage     string    `json:"service_language"`
	ServiceType         string    `json:"service_type"`
	StartTime           int64     `json:"start_time"`
	LastSeen            int64     `json:"last_seen"`
	APMInstrumentation  bool      `json:"apm_instrumentation"`
}

type event struct {
	RequestType eventType     `json:"request_type"`
	ApiVersion  string        `json:"api_version"`
	Payload     *eventPayload `json:"payload"`
}

type telemetrySender struct {
	sender   sender.Sender
	time     timer
	hostname hostname.Component
}

func (ts *telemetrySender) newEvent(t eventType, svc *serviceInfo) *event {
	host := ts.hostname.GetSafe(context.Background())
	env := pkgconfig.Datadog.GetString("env")

	return &event{
		RequestType: t,
		ApiVersion:  "v2",
		Payload: &eventPayload{
			ApiVersion:          "v1",
			NamingSchemaVersion: "1",
			RequestType:         t,
			ServiceName:         svc.meta.Name,
			HostName:            host,
			Env:                 env,
			ServiceLanguage:     svc.meta.Language,
			ServiceType:         svc.meta.Type,
			StartTime:           int64(svc.process.Stat.StartTime),
			LastSeen:            ts.time.Now().Unix(),
			APMInstrumentation:  false,
		},
	}
}

func newTelemetrySender(sender sender.Sender) *telemetrySender {
	return &telemetrySender{
		sender:   sender,
		time:     realTime{},
		hostname: hostnameimpl.NewHostnameService(),
	}
}

func (ts *telemetrySender) sendStartServiceEvent(svc *serviceInfo) {
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

func (ts *telemetrySender) sendHeartbeatServiceEvent(svc *serviceInfo) {
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

func (ts *telemetrySender) sendEndServiceEvent(svc *serviceInfo) {
	log.Debugf("[pid: %d | name: %s] stop-service",
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
