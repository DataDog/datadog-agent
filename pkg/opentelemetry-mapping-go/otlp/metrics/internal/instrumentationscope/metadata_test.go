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

package instrumentationscope

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTagsFromInstrumentationScopeMetadata(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		attrs        map[string]string
		expectedTags []string
	}{
		{
			"test-il", "1.0.0",
			nil,
			[]string{fmt.Sprintf("%s:%s", instrumentationScopeTag, "test-il"), fmt.Sprintf("%s:%s", instrumentationScopeVersionTag, "1.0.0")},
		},
		{
			"test-il", "",
			nil,
			[]string{fmt.Sprintf("%s:%s", instrumentationScopeTag, "test-il"), fmt.Sprintf("%s:%s", instrumentationScopeVersionTag, "n/a")},
		},
		{
			"", "1.0.0",
			nil,
			[]string{fmt.Sprintf("%s:%s", instrumentationScopeTag, "n/a"), fmt.Sprintf("%s:%s", instrumentationScopeVersionTag, "1.0.0")},
		},
		{
			"", "",
			nil,
			[]string{fmt.Sprintf("%s:%s", instrumentationScopeTag, "n/a"), fmt.Sprintf("%s:%s", instrumentationScopeVersionTag, "n/a")},
		},
		{
			"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc", "0.60.0",
			map[string]string{
				"otelcol.component.id":   "otlp",
				"otelcol.component.kind": "Receiver",
			},
			[]string{
				fmt.Sprintf("%s:%s", instrumentationScopeTag, "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"),
				fmt.Sprintf("%s:%s", instrumentationScopeVersionTag, "0.60.0"),
				"otelcol.component.id:otlp",
				"otelcol.component.kind:Receiver",
			},
		},
	}

	for _, testInstance := range tests {
		il := pcommon.NewInstrumentationScope()
		il.SetName(testInstance.name)
		il.SetVersion(testInstance.version)
		for k, v := range testInstance.attrs {
			il.Attributes().PutStr(k, v)
		}
		tags := TagsFromInstrumentationScopeMetadata(il)

		assert.ElementsMatch(t, testInstance.expectedTags, tags)
	}
}
