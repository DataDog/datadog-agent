// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package attributes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	conventions "go.opentelemetry.io/collector/model/semconv/v1.6.1"
)

func TestSystemExtractTags(t *testing.T) {
	sattrs := systemAttributes{
		OSType: "windows",
	}

	assert.Equal(t, []string{
		fmt.Sprintf("%s:%s", conventions.AttributeOSType, "windows"),
	}, sattrs.extractTags())
}

func TestSystemExtractTagsEmpty(t *testing.T) {
	sattrs := systemAttributes{}

	assert.Equal(t, []string{}, sattrs.extractTags())
}
