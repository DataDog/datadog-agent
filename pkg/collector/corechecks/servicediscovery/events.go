package servicediscovery

import (
	"context"
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"time"
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
	ServiceLanguage     int       `json:"service_language"`
	ServiceType         int       `json:"service_type"`
	Timestamp           int64     `json:"timestamp"`
	APMInstrumentation  bool      `json:"apm_instrumentation"`
}

type event struct {
	RequestType eventType     `json:"request_type"`
	ApiVersion  string        `json:"api_version"`
	Payload     *eventPayload `json:"payload"`
}

func newEvent(t eventType, p *processInfo) *event {
	h, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("failed to get hostname: %v", err)
	}

	return &event{
		RequestType: t,
		ApiVersion:  "v2",
		Payload: &eventPayload{
			ApiVersion:          "v1",
			NamingSchemaVersion: "1",
			RequestType:         t,
			ServiceName:         p.ShortName, // TODO
			HostName:            h,
			Env:                 pkgconfig.Datadog.GetString("env"),
			ServiceLanguage:     1, // TODO
			ServiceType:         2, // TODO
			Timestamp:           time.Now().Unix(),
			APMInstrumentation:  false,
		},
	}
}

type telemetrySender struct {
	sender sender.Sender
}

func newTelemetrySender(sender sender.Sender) *telemetrySender {
	return &telemetrySender{sender: sender}
}

// curl -X POST
// 'https://instrumentation-telemetry-intake.datad0g.com/api/v2/apmtelemetry'
// -H 'User-Agent:  '
// -H 'DD-API-KEY: MY_API_KEY'
// -H 'Content-Type: application/json; charset=utf-8'
//
//	-d '{
//			"request_type":"start-service",
//			"api_version":"v2",
//			"payload":{
//				"api_version":"v1",
//				"naming_schema_version":"1",
//				"request_type":"start-service",
//				"service_name":"my-service",
//				"host_name":"ec2-instance-0",
//				"env":"prod",
//				"service_language":1,
//				"service_type":2,
//				"timestamp":1712367374,
//				"apm_instrumentation":false
//			}
//		  }'
func (ts *telemetrySender) sendStartServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s | ports: %v] start-service",
		p.PID,
		p.ShortName,
		p.Ports,
	)

	e := newEvent(eventTypeStartService, p)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode start-service event as json: %v", err)
		return
	}
	log.Infof("sending start-service event: %s", string(b))

	// TODO: sender does not respond to SIGTERM when there are errors here
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendHeartbeatServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s] heartbeat-service",
		p.PID,
		p.ShortName,
	)

	e := newEvent(eventTypeHeartbeatService, p)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode heartbeat-service event as json: %v", err)
		return
	}
	log.Infof("sending heartbeat-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}

func (ts *telemetrySender) sendEndServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s] stop-service",
		p.PID,
		p.ShortName,
	)

	e := newEvent(eventTypeEndService, p)
	b, err := json.Marshal(e)
	if err != nil {
		log.Errorf("failed to encode end-service event as json: %v", err)
		return
	}
	log.Infof("sending end-service event: %s", string(b))
	ts.sender.EventPlatformEvent(b, eventplatform.EventTypeServiceDiscovery)
}
