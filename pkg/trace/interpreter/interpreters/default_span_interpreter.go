package interpreters

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"strings"
)

// DefaultSpanInterpreter sets up the default span interpreter
type DefaultSpanInterpreter struct {
	interpreter
}

// MakeDefaultSpanInterpreter creates an instance of the default span interpreter
func MakeDefaultSpanInterpreter(config *config.Config) *DefaultSpanInterpreter {
	return &DefaultSpanInterpreter{interpreter{Config: config}}
}

// Interpret performs the interpretation for the DefaultSpanInterpreter
func (in *DefaultSpanInterpreter) Interpret(span *pb.Span) *pb.Span {
	// no meta, add a empty map
	if span.Meta == nil {
		span.Meta = map[string]string{}
	}
	serviceName := in.ServiceName(span)
	span.Meta["span.serviceName"] = serviceName
	// create the service identifier using the already interpreted name
	span.Meta["span.serviceURN"] = in.CreateServiceURN(serviceName)
	return span
}

// ServiceName calculates a Service Name for this span given the interpreter config
func (in *DefaultSpanInterpreter) ServiceName(span *pb.Span) string {
	serviceNameSet := make([]string, 0)
	for _, identifier := range in.Config.ServiceIdentifiers {
		if identifierValue, found := span.Meta[identifier]; found {
			serviceNameSet = append(serviceNameSet, identifierValue)
		}
	}

	if len(serviceNameSet) > 0 {
		return fmt.Sprintf("%s:%s", span.Service, strings.Join(serviceNameSet, ":"))
	}

	return span.Service
}
