package interpreter

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSpanInterpreterEngine(t *testing.T) {
	sie := NewSpanInterpreterEngine(config.New())

	for _, tc := range []struct {
		testCase string
		span     pb.Span
		expected pb.Span
	}{
		{
			testCase: "Should run the default span interpreter if we have no metadata on the span",
			span:     pb.Span{Service: "SpanServiceName"},
			expected: pb.Span{
				Service: "SpanServiceName",
				Meta: map[string]string{
					"span.serviceName": "SpanServiceName",
					"span.serviceURN":  "urn:service:/SpanServiceName",
				},
			},
		},
		{
			testCase: "Should run the sql span interpreter if we have metadata and the type is 'sql'",
			span: pb.Span{
				Service: "Postgresql",
				Type:    "sql",
				Meta: map[string]string{
					"span.starttime": "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":  "hostname",
					"span.pid":       "10",
					"span.kind":      "some-kind",
					"db.type":        "postgresql",
					"db.instance":    "Instance",
				},
			},
			expected: pb.Span{
				Service: "Postgresql",
				Type:    "sql",
				Meta: map[string]string{
					"span.serviceName": "Postgresql:Instance",
					"span.starttime":   "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":    "hostname",
					"span.pid":         "10",
					"span.kind":        "some-kind",
					"db.type":          "postgresql",
					"db.instance":      "Instance",
					"span.serviceType": "postgresql",
					"span.serviceURN":  "urn:service:/Postgresql:Instance",
				},
			},
		},
		{
			testCase: "Should run the process span interpreter if we have metadata and the type is 'web'",
			span: pb.Span{
				Service: "WebServiceName",
				Type:    "web",
				Meta: map[string]string{
					"span.starttime": "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":  "hostname",
					"span.pid":       "10",
					"span.kind":      "some-kind",
				},
			},
			expected: pb.Span{
				Service: "WebServiceName",
				Type:    "web",
				Meta: map[string]string{
					"span.serviceName":        "WebServiceName",
					"span.starttime":          "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":           "hostname",
					"span.pid":                "10",
					"span.kind":               "some-kind",
					"span.serviceType":        "service",
					"span.serviceURN":         "urn:service:/WebServiceName",
					"span.serviceInstanceURN": "urn:service-instance:/WebServiceName:/hostname:10:1586441095",
				},
			},
		},
		{
			testCase: "Should run the process span interpreter if we have metadata and the type is 'server'",
			span: pb.Span{
				Service: "JavaServiceName",
				Type:    "server",
				Meta: map[string]string{
					"span.starttime": "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":  "hostname",
					"span.pid":       "10",
					"span.kind":      "some-kind",
					"language":       "jvm",
				},
			},
			expected: pb.Span{
				Service: "JavaServiceName",
				Type:    "server",
				Meta: map[string]string{
					"span.serviceName":        "JavaServiceName",
					"span.starttime":          "1586441095", //Thursday, 9 April 2020 14:04:55
					"span.hostname":           "hostname",
					"span.pid":                "10",
					"span.kind":               "some-kind",
					"language":                "jvm",
					"span.serviceType":        "java",
					"span.serviceURN":         "urn:service:/JavaServiceName",
					"span.serviceInstanceURN": "urn:service-instance:/JavaServiceName:/hostname:10:1586441095",
				},
			},
		},
		{
			testCase: "Should run the traefik span interpreter if the meta source is 'traefik'",
			span: pb.Span{
				Service: "TraefikServiceName",
				Meta: map[string]string{
					"source":    "traefik",
					"http.host": "hostname",
					"span.kind": "server",
				},
			},
			expected: pb.Span{
				Service: "TraefikServiceName",
				Meta: map[string]string{
					"span.serviceName": "hostname",
					"source":           "traefik",
					"http.host":        "hostname",
					"span.kind":        "server",
					"span.serviceType": "traefik",
					"span.serviceURN":  "urn:service:/hostname",
				},
			},
		},
		{
			testCase: "Should not interpret an already interpreted span",
			span: pb.Span{
				Service: "TraefikServiceName",
				Meta: map[string]string{
					"source":           "traefik",
					"http.host":        "hostname",
					"span.kind":        "server",
					"span.serviceType": "traefik",
					"span.serviceURN":  "some-different-external-urn-format",
				},
			},
			expected: pb.Span{
				Service: "TraefikServiceName",
				Meta: map[string]string{
					"source":           "traefik",
					"http.host":        "hostname",
					"span.kind":        "server",
					"span.serviceType": "traefik",
					"span.serviceURN":  "some-different-external-urn-format",
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			actual := sie.Interpret(&tc.span)
			assert.EqualValues(t, tc.expected, *actual)
		})
	}
}
