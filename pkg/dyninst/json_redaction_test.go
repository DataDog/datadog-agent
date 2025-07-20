// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

type matcher interface {
	matches(ptr jsontext.Pointer) bool
}

type replacer interface {
	replace(jsontext.Value) jsontext.Value
}

type replacerFunc func(jsontext.Value) jsontext.Value

func (f replacerFunc) replace(v jsontext.Value) jsontext.Value {
	return f(v)
}

type jsonRedactor struct {
	matcher  matcher
	replacer replacer
}

type exactMatcher string

func (m exactMatcher) matches(ptr jsontext.Pointer) bool {
	return string(ptr) == string(m)
}

type replacement jsontext.Value

func (r replacement) replace(jsontext.Value) jsontext.Value {
	return jsontext.Value(r)
}

type prefixSuffixMatcher [2]string

func (m prefixSuffixMatcher) matches(ptr jsontext.Pointer) bool {
	return strings.HasPrefix(string(ptr), m[0]) &&
		strings.HasSuffix(string(ptr), m[1])
}

func redactor(matcher matcher, replacer replacer) jsonRedactor {
	return jsonRedactor{
		matcher:  matcher,
		replacer: replacer,
	}
}

var defaultRedactors = []jsonRedactor{
	redactor(
		exactMatcher(`/debugger/snapshot/stack`),
		replacement(`"[stack-unredact-me]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/id`),
		replacement(`"[id]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		exactMatcher(`/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/address"},
		replacement(`"[addr]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/entries"},
		entriesSorter{},
	),
}

func redactJSON(t *testing.T, input []byte, redactors []jsonRedactor) (redacted []byte) {
	data := input

	// Apply each redactor in a separate pass
	for _, redactor := range redactors {
		d := jsontext.NewDecoder(bytes.NewReader(data))
		var buf bytes.Buffer
		e := jsontext.NewEncoder(&buf, jsontext.WithIndent("  "), jsontext.WithIndentPrefix("  "))

		for {
			tok, err := d.ReadToken()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			kind, idx := d.StackIndex(d.StackDepth())

			// Check if we're at a key position in an object
			if kind == '{' && idx%2 == 0 {
				// Write the key token
				err = e.WriteToken(tok)
				require.NoError(t, err)

				// Get the pointer that will be created when we're at the value
				ptr := d.StackPointer()

				if redactor.matcher.matches(ptr) {
					// This key matches our redaction pattern
					// Read and replace the value
					v, err := d.ReadValue()
					require.NoError(t, err)
					err = e.WriteValue(redactor.replacer.replace(v))
					require.NoError(t, err)
					continue
				}
			} else {
				// Default case: just write the token as-is
				err = e.WriteToken(tok)
				require.NoError(t, err)
			}
		}
		data = bytes.TrimSpace(buf.Bytes())
	}

	return data
}

type entriesSorter struct{}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var notCapturedReason = []byte(`"notCapturedReason"`)

func compareNotCapturedReason(a, b []byte) int {
	return cmp.Compare(
		boolToInt(bytes.Contains(a, notCapturedReason)),
		boolToInt(bytes.Contains(b, notCapturedReason)),
	)
}

func (e entriesSorter) replace(v jsontext.Value) jsontext.Value {
	var entries [][2]jsontext.Value
	if err := json.Unmarshal(v, &entries); err != nil {
		return v // Return original value if unmarshal fails
	}
	slices.SortFunc(entries, func(a, b [2]jsontext.Value) int {
		return cmp.Or(
			compareNotCapturedReason(a[0], b[0]),
			bytes.Compare(a[0], b[0]),
		)
	})
	sorted, err := json.Marshal(entries)
	if err != nil {
		return v // Return original value if marshal fails
	}
	return sorted
}

func TestDefaultRedactors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "redact snapshot id",
			input: `{
  "debugger": {
    "snapshot": {
      "id": "actual-snapshot-id-123"
    }
  }
}`,
			expected: `{
    "debugger": {
      "snapshot": {
        "id": "[id]"
      }
    }
  }`,
		},
		{
			name: "redact snapshot timestamp",
			input: `{
  "debugger": {
    "snapshot": {
      "timestamp": "[ts]"
    }
  }
}`,
			expected: `{
    "debugger": {
      "snapshot": {
        "timestamp": "[ts]"
      }
    }
  }`,
		},
		{
			name: "redact root timestamp",
			input: `{
  "timestamp": "2023-01-01T00:00:00Z",
  "other": "data"
}`,
			expected: `{
    "timestamp": "[ts]",
    "other": "data"
  }`,
		},
		{
			name: "redact snapshot stack",
			input: `{
  "debugger": {
    "snapshot": {
      "stack": ["frame1", "frame2", "frame3"]
    }
  }
}`,
			expected: `{
    "debugger": {
      "snapshot": {
        "stack": "[stack-unredact-me]"
      }
    }
  }`,
		},
		{
			name: "redact capture address",
			input: `{
  "debugger": {
    "snapshot": {
      "captures": {
        "locals": {
          "address": "0x7fff5fbff123"
        }
      }
    }
  }
}`,
			expected: `{
    "debugger": {
      "snapshot": {
        "captures": {
          "locals": {
            "address": "[addr]"
          }
        }
      }
    }
  }`,
		},
		{
			name: "sort capture entries",
			input: `{
  "debugger": {
    "snapshot": {
      "captures": {
        "locals": {
          "entries": [
            ["variable_z", {"value": "z"}],
            ["variable_a", {"value": "a"}],
            ["variable_m", {"notCapturedReason": "error"}]
          ]
        }
      }
    }
  }
}`,
			expected: `{
    "debugger": {
      "snapshot": {
        "captures": {
          "locals": {
            "entries": [
              ["variable_a", {"value": "a"}],
              ["variable_m", {"notCapturedReason": "error"}],
              ["variable_z", {"value": "z"}]
            ]
          }
        }
      }
    }
  }`,
		},
		{
			name: "multiple redactions",
			input: `{
  "timestamp": "2023-01-01T00:00:00Z",
  "debugger": {
    "snapshot": {
      "id": "snap-123",
      "timestamp": "2023-01-01T01:00:00Z",
      "stack": ["frame1", "frame2"],
      "captures": {
        "locals": {
          "address": "0x123456",
          "entries": [
            ["var_b", {"value": "b"}],
            ["var_a", {"value": "a"}]
          ]
        }
      }
    }
  }
}`,
			expected: `{
    "timestamp": "[ts]",
    "debugger": {
      "snapshot": {
        "id": "[id]",
        "timestamp": "[ts]",
        "stack": "[stack-unredact-me]",
        "captures": {
          "locals": {
            "address": "[addr]",
            "entries": [
              ["var_a", {"value": "a"}],
              ["var_b", {"value": "b"}]
            ]
          }
        }
      }
    }
  }`,
		},
		{
			name: "no redactions needed",
			input: `{
  "normal": "field",
  "data": {
    "value": 123
  }
}`,
			expected: `{
    "normal": "field",
    "data": {
      "value": 123
    }
  }`,
		},
		{
			name: "map with addr",
			input: `{
  "timestamp": "2023-01-01T00:00:00Z",
  "debugger": {
    "snapshot": {
      "id": "snap-123",
      "timestamp": "2023-01-01T01:00:00Z",
      "stack": ["frame1", "frame2"],
      "captures": {
        "entry": {
		  "arguments": {
		    "m": {
			  "type": "map[string]main.bigStruct",
			  "address": "0x123456",
			  "entries": [
				[ {"key": "k1", "value": {"a": 1, "b": 2}}, {"key": "k2", "value": {"a": 3, "b": 4}} ]
	          ]
			}
		  }
        }
      }
    }
  }
}`,
			expected: `{
  "timestamp": "[ts]",
  "debugger": {
    "snapshot": {
      "id": "[id]",
      "timestamp": "[ts]",
      "stack": "[stack-unredact-me]",
      "captures": {
        "entry": {
		  "arguments": {
		    "m": {
			  "type": "map[string]main.bigStruct",
			  "address": "[addr]",
			  "entries": [
				[ {"key": "k1", "value": {"a": 1, "b": 2}}, {"key": "k2", "value": {"a": 3, "b": 4}} ]
	          ]
			}
		  }
        }
      }
    }
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactJSON(t, []byte(tt.input), defaultRedactors)
			require.JSONEq(t, tt.expected, string(result), "redacted JSON should match expected output")
		})
	}
}
