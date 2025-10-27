// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"testing"
)

func TestFormatKeyValueTag(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "normal key value",
			key:      "service",
			value:    "web",
			expected: "service:web",
		},
		{
			name:     "empty value",
			key:      "env",
			value:    "",
			expected: "env:n/a",
		},
		{
			name:     "empty key",
			key:      "",
			value:    "value",
			expected: ":value",
		},
		{
			name:     "both empty",
			key:      "",
			value:    "",
			expected: ":n/a",
		},
		{
			name:     "long key and value",
			key:      "very_long_service_name",
			value:    "very_long_value_name",
			expected: "very_long_service_name:very_long_value_name",
		},
		{
			name:     "special characters in key",
			key:      "service-name",
			value:    "web-app",
			expected: "service-name:web-app",
		},
		{
			name:     "special characters in value",
			key:      "env",
			value:    "prod-1",
			expected: "env:prod-1",
		},
		{
			name:     "unicode characters",
			key:      "service",
			value:    "测试",
			expected: "service:测试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatKeyValueTag(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("FormatKeyValueTag(%q, %q) = %q, want %q", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}
