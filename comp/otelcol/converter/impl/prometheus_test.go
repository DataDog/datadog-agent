// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package converterimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
)

func TestFindInternalMetricsAddress(t *testing.T) {
	tests := []struct {
		host   any
		port   any
		result string
	}{
		{
			host:   nil,
			port:   9999,
			result: "0.0.0.0:9999",
		},
		{
			host:   nil,
			port:   nil,
			result: "0.0.0.0:8888",
		},
		{
			host:   "1.2.3.4",
			port:   nil,
			result: "1.2.3.4:8888",
		},
		{
			host:   "1.2.3.4",
			port:   "9999",
			result: "1.2.3.4:8888",
		},
		{
			host:   5,
			port:   9999,
			result: "0.0.0.0:9999",
		},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("findInternalMetricsAddress(%v,%v)", tc.host, tc.port)

		t.Run(name, func(t *testing.T) {
			conf := confmap.NewFromStringMap(map[string]any{
				"service": map[string]any{
					"telemetry": map[string]any{
						"metrics": map[string]any{
							"readers": []any{
								map[string]any{
									"pull": map[string]any{
										"exporter": map[string]any{
											"prometheus": map[string]any{
												"host": tc.host,
												"port": tc.port,
											},
										},
									},
								},
							},
						},
					},
				},
			})

			addr := findInternalMetricsAddress(conf)
			assert.Equal(t, tc.result, addr)
		})
	}

}
