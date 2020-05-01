package interpreters

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
)

// ProcessSpanInterpreter sets up the process span interpreter
type ProcessSpanInterpreter struct {
	interpreter
}

// ProcessSpanInterpreterName is the name used for matching this interpreter
const ProcessSpanInterpreterName = "process"

// ProcessTypeName returns the default process type
const ProcessTypeName = "process"

// MakeProcessSpanInterpreter creates an instance of the process span interpreter
func MakeProcessSpanInterpreter(config *config.Config) *ProcessSpanInterpreter {
	return &ProcessSpanInterpreter{interpreter{Config: config}}
}

// Interpret performs the interpretation for the ProcessSpanInterpreter
func (in *ProcessSpanInterpreter) Interpret(span *model.SpanWithMeta) *pb.Span {
	serviceType := ServiceTypeName

	// no meta, add a empty map
	if span.Meta == nil {
		span.Meta = map[string]string{}
	}

	if language, found := span.Meta["language"]; found {
		serviceType = in.LanguageToComponentType(language)
	}
	span.Meta["span.serviceType"] = serviceType

	// create the service instance identifier using the already interpreted name
	span.Meta["span.serviceInstanceURN"] = in.CreateServiceInstanceURN(span.Meta["span.serviceName"], span.Hostname, span.PID, span.CreateTime)

	return span.Span
}

// LanguageToComponentType converts a trace language to a component type
func (in *ProcessSpanInterpreter) LanguageToComponentType(spanLanguage string) string {
	switch spanLanguage {
	case "jvm":
		return "java"
	default:
		return ProcessTypeName
	}
}
