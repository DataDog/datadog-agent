package interpreters

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
)

// ServiceTypeName returns the default service type
const ServiceTypeName = "service"

// ServiceInstanceTypeName returns the default service instance type
const ServiceInstanceTypeName = "service-instance"

// SourceInterpreter provides the interface for the different source interpreters
type SourceInterpreter interface {
	Interpret(span *pb.Span) *pb.Span
}

// TypeInterpreter provides the interface for the different type interpreters
type TypeInterpreter interface {
	Interpret(span *model.SpanWithMeta) *pb.Span
}

type interpreter struct {
	Config *config.Config
}

// CreateServiceURN creates the urn identifier for all service components
func (i *interpreter) CreateServiceURN(serviceName string) string {
	return fmt.Sprintf("urn:%s:/%s", ServiceTypeName, serviceName)
}

// CreateServiceInstanceURN creates the urn identifier for all service instance components
func (i *interpreter) CreateServiceInstanceURN(serviceName string, hostname string, pid int, createTime int64) string {
	return fmt.Sprintf("urn:%s:/%s:/%s:%d:%d", ServiceInstanceTypeName, serviceName, hostname, pid, createTime)
}
