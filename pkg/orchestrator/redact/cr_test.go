// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

//goland:noinspection ALL
var avoidOptUnstructured *unstructured.Unstructured

func BenchmarkScrubCRManifest1(b *testing.B)    { benchmarkScrubCRManifest(1, b) }
func BenchmarkScrubCRManifest10(b *testing.B)   { benchmarkScrubCRManifest(10, b) }
func BenchmarkScrubCRManifest100(b *testing.B)  { benchmarkScrubCRManifest(100, b) }
func BenchmarkScrubCRManifest1000(b *testing.B) { benchmarkScrubCRManifest(1000, b) }

func benchmarkScrubCRManifest(nbCustomResources int, b *testing.B) {
	customResourcesBenchmarks := make([]*unstructured.Unstructured, nbCustomResources)
	customResourcesToBenchmark := make([]*unstructured.Unstructured, nbCustomResources)
	c := &unstructured.Unstructured{}

	// Initialize the slices with empty maps to avoid nil pointer dereferences
	for i := range nbCustomResources {
		customResourcesBenchmarks[i] = &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}
		customResourcesToBenchmark[i] = &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}
	}

	scrubber := NewDefaultDataScrubber()
	for _, testCase := range getCRScrubCases() {
		customResourcesToBenchmark = append(customResourcesToBenchmark, testCase.input)
	}
	for i := 0; i < nbCustomResources; i++ {
		customResourcesBenchmarks = append(customResourcesBenchmarks, customResourcesToBenchmark...)
	}
	b.ResetTimer()

	b.Run("simplified", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range customResourcesBenchmarks {
				ScrubCRManifest(c, scrubber)
			}
		}
	})
	avoidOptUnstructured = c
}
func TestScrubMapWithoutParentSensitive(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	m := map[string]interface{}{
		"password": "abcdef",
		"dog":      "cat",
	}
	scrubMap(m, scrubber, false)
	assert.Equal(t, map[string]interface{}{
		"password": "********",
		"dog":      "cat",
	}, m)
}

func TestScrubMapWithParentSensitive(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	m := map[string]interface{}{
		"password": "abcdef",
		"dog":      "cat",
	}
	scrubMap(m, scrubber, true)
	assert.Equal(t, map[string]interface{}{
		"password": "********",
		"dog":      "********",
	}, m)
}

func TestScrubSliceWithoutParentSensitive(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	slice := []interface{}{
		"password=abc",
		"dog",
	}
	scrubSlice(slice, scrubber, false)
	assert.Equal(t, []interface{}{
		"********",
		"dog",
	}, slice)
}

func TestScrubSliceWithParentSensitive(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	slice := []interface{}{
		"cat",
		"dog",
	}
	scrubSlice(slice, scrubber, true)
	assert.Equal(t, []interface{}{
		"********",
		"********",
	}, slice)
}

func TestScrubEnv(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	env := []interface{}{
		map[string]interface{}{
			"name":  "name",
			"value": "cat",
		},
		map[string]interface{}{
			"name":  "API_KEY",
			"value": "secret1",
		},
	}
	scrubEnv(env, scrubber, false)
	assert.Equal(t, []interface{}{
		map[string]interface{}{
			"name":  "name",
			"value": "cat",
		},
		map[string]interface{}{
			"name":  "API_KEY",
			"value": "********",
		},
	}, env)
}

func TestScrubCRManifest(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	tests := getCRScrubCases()
	scrubber.AddCustomSensitiveWords([]string{"color"})

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ScrubCRManifest(tc.input, scrubber)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}
func getCRScrubCases() map[string]struct {
	input    *unstructured.Unstructured
	expected *unstructured.Unstructured
} {
	tests := map[string]struct {
		input    *unstructured.Unstructured
		expected *unstructured.Unstructured
	}{
		"custom sensitive word": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"color": "red",
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"color": "********",
					},
				},
			},
		},
		"sensitive slice": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": []interface{}{"mypassword", "supersecret", "afztyerbzio1234"},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": []interface{}{"********", "********", "********"},
					},
				},
			},
		},
		"sensitive map": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": "mypassword",
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": "********",
					},
				},
			},
		},
		"sensitive env": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{
								"name":  "password",
								"value": "mypassword",
							},
							map[string]interface{}{
								"name":  "API_KEY",
								"value": "secret1",
							},
						},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{
								"name":  "password",
								"value": "********",
							},
							map[string]interface{}{
								"name":  "API_KEY",
								"value": "********",
							},
						},
					},
				},
			},
		},
		"sensitive parent with nested map": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"map": map[string]interface{}{
								"val": "mypassword",
							},
						},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"map": map[string]interface{}{
								"val": "********",
							},
						},
					},
				},
			},
		},
		"sensitive parent with nested slice": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"slice": []interface{}{
								"item",
								"item2",
							},
						},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"slice": []interface{}{
								"********",
								"********",
							},
						},
					},
				},
			},
		},
		"sensitive parent with nested env": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"env": []interface{}{
								map[string]interface{}{
									"name":  "name",
									"value": "cat",
								},
								map[string]interface{}{
									"name":  "DD_NAME",
									"value": "dog",
								},
							},
						},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"credentials": map[string]interface{}{
							"env": []interface{}{
								map[string]interface{}{
									"name":  "name",
									"value": "********",
								},
								map[string]interface{}{
									"name":  "DD_NAME",
									"value": "********",
								},
							},
						},
					},
				},
			},
		},
		"unexcepted env type": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{"cat", "dog"},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{"cat", "dog"},
					},
				},
			},
		},
		"env missing value or name": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{
								"name": "password",
							},
							map[string]interface{}{
								"value": "cat",
							},
						},
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{
								"name": "password",
							},
							map[string]interface{}{
								"value": "cat",
							},
						},
					},
				},
			},
		},
		"interger value": {
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": 123,
					},
				},
			},
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"password": 123,
					},
				},
			},
		},
	}
	return tests
}
