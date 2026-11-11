// Copyright OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package attributes

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv143 "go.opentelemetry.io/otel/semconv/v1.43.0"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

func hasFaaSAttribute(attrs pcommon.Map) bool {
	found := false
	attrs.Range(func(key string, _ pcommon.Value) bool {
		found = strings.HasPrefix(key, "faas.")
		return !found
	})
	return found
}

func awsLambdaSourceFromAttributes(attrs pcommon.Map) (source.Source, bool) {
	if stringAttribute(attrs, string(conventions.CloudProviderKey)) != conventions.CloudProviderAWS.Value.AsString() {
		return source.Source{}, false
	}

	platform := stringAttribute(attrs, string(conventions.CloudPlatformKey))
	faaSID := stringAttribute(attrs, string(conventions.FaaSIDKey))
	resourceID := stringAttribute(attrs, string(semconv143.CloudResourceIDKey))
	functionARN := faaSID
	if functionARN == "" {
		functionARN = resourceID
	}
	if platform != "aws_lambda" && faaSID == "" && (resourceID == "" || !hasFaaSAttribute(attrs)) {
		return source.Source{}, false
	}

	parts := strings.Split(functionARN, ":")
	if len(parts) < 7 || parts[0] != "arn" || parts[2] != "lambda" || parts[5] != "function" {
		return source.Source{}, false
	}
	normalizedARN := strings.Join(parts[:7], ":")

	return source.Source{
		Kind: source.AWSLambdaKind,
		SourceIdentifier: source.SourceIdentifier{
			Primary: normalizedARN,
			Dimensions: map[string]string{
				"function_arn":  normalizedARN,
				"function_name": parts[6],
				"region":        parts[3],
				"aws_account":   parts[4],
			},
		},
	}, true
}
