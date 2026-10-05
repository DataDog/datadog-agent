// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package converters

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var defaultHTTPTestTelemetry = []prometheusTelemetryTarget{
	{HostPort: defaultTelemetryReaderTarget, Scheme: "http"},
}

func TestTelemetryTargets_NoService(t *testing.T) {
	targets, err := selectTelemetryPrometheusTargets(confMap{})
	require.NoError(t, err)
	require.Equal(t, defaultHTTPTestTelemetry, targets)
}

func TestTelemetryTargets_AbsentReaders(t *testing.T) {
	conf := confMap{
		"service": confMap{
			"telemetry": confMap{
				"metrics": confMap{},
			},
		},
	}
	targets, err := selectTelemetryPrometheusTargets(conf)
	require.NoError(t, err)
	require.Equal(t, defaultHTTPTestTelemetry, targets)
}

func TestTelemetryTargets_EmptyReaders(t *testing.T) {
	conf := confMap{
		"service": confMap{
			"telemetry": confMap{
				"metrics": confMap{
					"readers": []any{},
				},
			},
		},
	}
	targets, err := selectTelemetryPrometheusTargets(conf)
	require.ErrorIs(t, err, errInvalidTelemetryMetricsReaders)
	require.Nil(t, targets)
}

func TestTelemetryTargets_PullReader(t *testing.T) {
	conf := confMap{
		"service": confMap{
			"telemetry": confMap{
				"metrics": confMap{
					"readers": []any{
						confMap{
							"pull": confMap{
								"exporter": confMap{
									"prometheus": confMap{
										"host": "example.test",
										"port": 9999,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	targets, err := selectTelemetryPrometheusTargets(conf)
	require.NoError(t, err)
	require.Equal(t, []prometheusTelemetryTarget{{HostPort: "example.test:9999", Scheme: "http"}}, targets)
}

func TestCoverage_TargetMismatch(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"static_configs": []any{
								confMap{
									"targets": []any{"127.0.0.1:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Empty(t, covered)
}

func TestCoverage_TargetMatch(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Contains(t, covered, "otlp_http")
}

func TestCoverage_CustomPath(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"metrics_path": "/custom",
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Empty(t, covered)
}

func TestCoverage_MetricsPath(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"metrics_path": "/metrics",
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Contains(t, covered, "otlp_http")
}

func TestCoverage_HTTPScheme(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"scheme": "https",
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Empty(t, covered)
}

func TestCoverage_BadScrapeConfigsType(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": "not-a-slice",
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidPrometheusReceiver)
	require.Nil(t, covered)
}

func TestCoverage_BadMetricsPathType(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"metrics_path": 42,
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.Error(t, err)
	require.ErrorIs(t, err, errInvalidPrometheusReceiver)
	require.Nil(t, covered)
}

func TestNormalizeMetricsPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		scrape confMap
		want   string
		wantOK bool
	}{
		{
			name:   "absent",
			scrape: confMap{},
			want:   prometheusDefaultMetricsPath,
			wantOK: true,
		},
		{
			name:   "empty",
			scrape: confMap{"metrics_path": ""},
			want:   prometheusDefaultMetricsPath,
			wantOK: true,
		},
		{
			name:   "trimmed",
			scrape: confMap{"metrics_path": "  /metrics  "},
			want:   otelDefaultMetricsPath,
			wantOK: true,
		},
		{
			name:   "tricky path",
			scrape: confMap{"metrics_path": "/foo/../metrics"},
			want:   otelDefaultMetricsPath,
			wantOK: true,
		},
		{
			name:   "custom",
			scrape: confMap{"metrics_path": "/custom"},
			want:   "/custom",
			wantOK: true,
		},
		{
			name:   "wrong type",
			scrape: confMap{"metrics_path": 42},
			want:   "",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := normalizePrometheusMetricsPath(tc.scrape)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCoverage_SchemeTrimCase(t *testing.T) {
	conf := confMap{
		"receivers": confMap{
			"prometheus/user": confMap{
				"config": confMap{
					"scrape_configs": []any{
						confMap{
							"scheme": "  HTTP  ",
							"static_configs": []any{
								confMap{
									"targets": []any{"localhost:8888"},
								},
							},
						},
					},
				},
			},
		},
		"service": confMap{
			"pipelines": confMap{
				"metrics/user": confMap{
					"receivers": []any{"prometheus/user"},
					"exporters": []any{"otlp_http"},
				},
			},
		},
	}
	covered, err := getCoveredExportersInMetricsPipelines(conf, defaultHTTPTestTelemetry, isComponentTypeOtlpHTTP)
	require.NoError(t, err)
	require.Contains(t, covered, "otlp_http")
}
