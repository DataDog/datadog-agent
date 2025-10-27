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
			name:     "normal key value pair",
			key:      "service",
			value:    "api",
			expected: "service:api",
		},
		{
			name:     "empty value should be replaced with n/a",
			key:      "version",
			value:    "",
			expected: "version:n/a",
		},
		{
			name:     "empty key with value",
			key:      "",
			value:    "test",
			expected: ":test",
		},
		{
			name:     "both empty",
			key:      "",
			value:    "",
			expected: ":n/a",
		},
		{
			name:     "special characters in key and value",
			key:      "env:prod",
			value:    "region:us-east-1",
			expected: "env:prod:region:us-east-1",
		},
		{
			name:     "long key and value",
			key:      "very-long-key-name-that-might-cause-allocations",
			value:    "very-long-value-that-might-cause-allocations",
			expected: "very-long-key-name-that-might-cause-allocations:very-long-value-that-might-cause-allocations",
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
