package servicediscovery

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventStartService struct {
}

type eventStopService struct {
}

type eventHeartbeatService struct {
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
func (c *Check) startServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s | ports: %v] start-service",
		p.PID,
		p.ShortName,
		p.Ports,
	)
	// c.sender.EventPlatformEvent(nil, eventplatform.EventTypeServiceDiscovery)
}

func (c *Check) heartbeatServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s] heartbeat-service",
		p.PID,
		p.ShortName,
	)
}

func (c *Check) stopServiceEvent(p *processInfo) {
	log.Infof("[pid: %d | name: %s] stop-service",
		p.PID,
		p.ShortName,
	)
}
