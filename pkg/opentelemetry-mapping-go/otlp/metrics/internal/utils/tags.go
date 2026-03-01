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

// FormatKeyValueTag takes a key-value pair, and creates a tag string out of it
// Tags can't end with ":" so we replace empty values with "n/a"
func FormatKeyValueTag(key, value string) string {
	if value == "" {
		value = "n/a"
	}
	// Pre-size the buffer: key + ":" + value
	b := make([]byte, 0, len(key)+1+len(value))
	b = append(b, key...)
	b = append(b, ':')
	b = append(b, value...)
	return string(b)
}
