package writer

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// ServiceWriter ...
type ServiceWriter struct{}

// NewServiceWriter ...
func NewServiceWriter(_ *config.AgentConfig, in chan pb.ServicesMetadata) *ServiceWriter {
	go func() {
		for {
			<-in
		}
	}()
	return new(ServiceWriter)
}

// Start ...
func (*ServiceWriter) Start() {}

// Stop ...
func (*ServiceWriter) Stop() {}
