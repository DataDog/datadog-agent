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

// Package utils provides utilities for the OpenTelemetry Collector.
package utils

import (
	"strings"
)

// FormatKeyValueTag takes a key-value pair, and creates a tag string out of it
// Tags can't end with ":" so we replace empty values with "n/a"
func FormatKeyValueTag(key, value string) string {
	if value == "" {
		value = "n/a"
	}
	// Pre-allocate buffer with known capacity to avoid multiple allocations
	var builder strings.Builder
	builder.Grow(len(key) + len(value) + 1) // +1 for the colon
	builder.WriteString(key)
	builder.WriteString(":")
	builder.WriteString(value)
	return builder.String()
}
