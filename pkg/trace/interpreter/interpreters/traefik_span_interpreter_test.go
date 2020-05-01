package interpreters

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTraefikSpanInterpreter(t *testing.T) {
	traefikInterpreter := MakeTraefikInterpreter(config.DefaultInterpreterConfig())
	for _, tc := range []struct {
		testCase    string
		interpreter *TraefikInterpreter
		span        pb.Span
		expected    pb.Span
	}{
		{
			testCase:    "Should set span.serviceType to 'traefik' when no span.kind metadata exists",
			interpreter: traefikInterpreter,
			span:        pb.Span{Service: "service-name"},
			expected: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceType": "traefik",
				},
			},
		},
		{
			testCase:    "Should set name and service to 'http.host' when span.kind is 'server'",
			interpreter: traefikInterpreter,
			span: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"http.host": "hostname",
					"span.kind": "server",
				},
			},
			expected: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceName": "hostname",
					"span.serviceURN":  "urn:service:/hostname",
					"http.host":        "hostname",
					"span.kind":        "server",
					"span.serviceType": "traefik",
				},
			},
		},
		{
			testCase:    "Should set name and service to 'http.host' when span.kind is 'client'",
			interpreter: traefikInterpreter,
			span: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"backend.name": "backend-service-name",
					"span.kind":    "client",
				},
			},
			expected: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceName": "service-name",
					"span.serviceURN":  "urn:service:/service-name",
					"backend.name":     "backend-service-name",
					"span.kind":        "client",
					"span.serviceType": "traefik",
				},
			},
		},
		{
			testCase:    "Should create a service instance identifier with the 'http.url' host when span.kind is 'client'",
			interpreter: traefikInterpreter,
			span: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"backend.name":     "backend-service-name",
					"span.kind":        "client",
					"span.serviceName": "service-name",
					"http.url":         "https://myhost.com:8080/some/path",
				},
			},
			expected: pb.Span{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceName":         "service-name",
					"backend.name":             "backend-service-name",
					"http.url":                 "https://myhost.com:8080/some/path",
					"span.kind":                "client",
					"span.serviceType":         "traefik",
					"span.serviceURN":          "urn:service:/service-name",
					"span.serviceInstanceURN":  "urn:service-instance:/service-name:/myhost.com",
					"span.serviceInstanceHost": "myhost.com",
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			actual := tc.interpreter.Interpret(&tc.span)
			assert.EqualValues(t, tc.expected, *actual)
		})
	}
}
