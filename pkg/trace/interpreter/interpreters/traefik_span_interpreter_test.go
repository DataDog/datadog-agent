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
		trace       []*pb.Span
		expected    []*pb.Span
	}{
		{
			testCase:    "Should set span.serviceType to 'traefik' when no span.kind metadata exists",
			interpreter: traefikInterpreter,
			trace:       []*pb.Span{{Service: "service-name"}},
			expected: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceType": "traefik",
				},
			}},
		},
		{
			testCase:    "Should set name and service to 'http.host' when span.kind is 'server'",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"http.host": "hostname",
					"span.kind": "server",
				},
			}},
			expected: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceName": "hostname",
					"span.serviceURN":  "urn:service:/hostname",
					"http.host":        "hostname",
					"span.kind":        "server",
					"span.serviceType": "traefik",
				},
			}},
		},
		{
			testCase:    "Should set name and service to 'http.host' when span.kind is 'client'",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"backend.name": "backend-service-name",
					"span.kind":    "client",
				},
			}},
			expected: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"span.serviceName": "service-name",
					"span.serviceURN":  "urn:service:/service-name",
					"backend.name":     "backend-service-name",
					"span.kind":        "client",
					"span.serviceType": "traefik",
				},
			}},
		},
		{
			testCase:    "Should create a service instance identifier with the 'http.url' host when span.kind is 'client'",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{{
				Service: "service-name",
				Meta: map[string]string{
					"backend.name":     "backend-service-name",
					"span.kind":        "client",
					"span.serviceName": "service-name",
					"http.url":         "https://myhost.com:8080/some/path",
				},
			}},
			expected: []*pb.Span{{
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
			}},
		},
		{
			testCase:    "Should rewrite the trace so that the forward span is the child of the entrupoint span",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{
				{
					SpanID:  1,
					Service: "entrypoint-service",
					Meta: map[string]string{
						"http.host": "hostname",
						"span.kind": "server",
					},
				},
				{
					SpanID:   2,
					ParentID: 1,
					Name:     "TLSClientHeaders",
					Service:  "TraefikService",
					Meta: map[string]string{
						"source": "traefik",
					},
				},
				{
					SpanID:   3,
					ParentID: 2,
					Service:  "forward-service",
					Meta: map[string]string{
						"backend.name": "backend-service-name",
						"span.kind":    "client",
					},
				},
			},
			expected: []*pb.Span{
				{
					SpanID:  1,
					Service: "entrypoint-service",
					Meta: map[string]string{
						"http.host":        "hostname",
						"span.kind":        "server",
						"span.serviceName": "hostname",
						"span.serviceURN":  "urn:service:/hostname",
						"span.serviceType": "traefik",
					},
				},
				{
					SpanID:   2,
					ParentID: 1,
					Name:     "TLSClientHeaders",
					Service:  "TraefikService",
					Meta: map[string]string{
						"source":           "traefik",
						"span.serviceType": "traefik",
					},
				},
				{
					SpanID:   3,
					ParentID: 1,
					Service:  "forward-service",
					Meta: map[string]string{
						"backend.name":     "backend-service-name",
						"span.kind":        "client",
						"span.serviceName": "service-name",
						"span.serviceURN":  "urn:service:/service-name",
						"span.serviceType": "traefik",
					},
				},
			},
		},
		{
			testCase:    "Should interpret 4xx http errors",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{{
				Service: "service-name",
				Error:   1,
				Metrics: map[string]float64{
					"http.status_code": 404.0,
				},
			}},
			expected: []*pb.Span{{
				Service: "service-name",
				Error:   1,
				Metrics: map[string]float64{
					"http.status_code": 404.0,
				},
				Meta: map[string]string{
					"span.serviceType": "traefik",
					"span.errorClass":  "4xx",
				},
			}},
		},
		{
			testCase:    "Should interpret 5xx http errors",
			interpreter: traefikInterpreter,
			trace: []*pb.Span{{
				Service: "service-name",
				Error:   1,
				Metrics: map[string]float64{
					"http.status_code": 503.0,
				},
			}},
			expected: []*pb.Span{{
				Service: "service-name",
				Error:   1,
				Metrics: map[string]float64{
					"http.status_code": 503.0,
				},
				Meta: map[string]string{
					"span.serviceType": "traefik",
					"span.errorClass":  "5xx",
				},
			}},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			actual := tc.interpreter.Interpret(tc.trace)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
