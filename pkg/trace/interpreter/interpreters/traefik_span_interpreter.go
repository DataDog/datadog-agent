package interpreters

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"net/url"
	"strings"
)

// TraefikInterpreter sets up the default span interpreter
type TraefikInterpreter struct {
	interpreter
}

// TraefikSpanInterpreterSpan is the name used for matching this interpreter
const TraefikSpanInterpreterSpan = "traefik"

// MakeTraefikInterpreter creates an instance of the traefik span interpreter
func MakeTraefikInterpreter(config *config.Config) *TraefikInterpreter {
	return &TraefikInterpreter{interpreter{Config: config}}
}

// Interpret performs the interpretation for the TraefikInterpreter
func (t *TraefikInterpreter) Interpret(spans []*pb.Span) []*pb.Span {
	// In a Traefik trace we will always find 3 spans:
	//  entrypoint -> TLS headers -> forward
	// We interpret:
	// - the entrypoint as the frontend service
	// - the TLS header span as being the core Traefik component
	// - the forward span as being the backend service and service instance component
	//   they will both merge eventually with other components in StackState
	var entrypoint, forward *pb.Span
	for _, span := range spans {
		// no meta, add a empty map
		if span.Meta == nil {
			span.Meta = map[string]string{}
		}

		if kind, found := span.Meta["span.kind"]; found {
			switch kind {
			case "server":
				entrypoint = span

				// this is the calling service, take the host as identifier
				// e.g. urn:service:/api-service-router.staging.furby.ps
				if host, found := entrypoint.Meta["http.host"]; found {
					entrypoint.Meta["span.serviceURN"] = t.CreateServiceURN(host)
					entrypoint.Meta["span.serviceName"] = host
				}
			case "client":
				forward = span

				// this is the called service, takes the backend.name as identifier
				// e.g. "backend-stackstate-books-app" -> urn:service:/stackstate-books-app
				if backendName, found := forward.Meta["backend.name"]; found {
					backendName = strings.TrimPrefix(backendName, "backend-")
					forward.Meta["span.serviceURN"] = t.CreateServiceURN(backendName)
					forward.Meta["span.serviceName"] = backendName
				}

				// create the service instance identifier using the already interpreted name and the host extracted from the meta "http.url"
				if urlString, found := forward.Meta["http.url"]; found {
					parsedURL, err := url.Parse(urlString)
					if err == nil {
						forward.Meta["span.serviceInstanceURN"] = t.CreateServiceInstanceURN(forward.Meta["span.serviceName"], parsedURL.Hostname())
						forward.Meta["span.serviceInstanceHost"] = parsedURL.Hostname()
					}
				}
			}
		}

		t.interpretHTTPError(span)

		span.Meta["span.serviceType"] = "traefik"
	}

	// change parent id for forward span to entrypoint id
	if forward != nil && entrypoint != nil {
		forward.ParentID = entrypoint.SpanID
	}

	return spans
}

// CreateServiceInstanceURN creates the urn identifier for all traefik service instance components
func (t *TraefikInterpreter) CreateServiceInstanceURN(serviceName string, hostname string) string {
	return fmt.Sprintf("urn:%s:/%s:/%s", ServiceInstanceTypeName, serviceName, hostname)
}

func (t *TraefikInterpreter) interpretHTTPError(span *pb.Span) {
	if span.Error != 0 {
		if httpStatus, found := span.Metrics["http.status_code"]; found {
			if httpStatus >= 400 && httpStatus < 500 {
				span.Meta["span.errorClass"] = "4xx"
			} else if httpStatus >= 500 {
				span.Meta["span.errorClass"] = "5xx"
			}
		}
	}
}
