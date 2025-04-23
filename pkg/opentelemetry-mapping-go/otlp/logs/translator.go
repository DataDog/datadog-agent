// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.opentelemetry.io/otel/attribute"
)

var (
	signalTypeSet = attribute.NewSet(attribute.String("signal", "logs"))
)

// Translator of OTLP logs to Datadog format
type Translator struct {
	set                  component.TelemetrySettings
	attributesTranslator *attributes.Translator
	otelTag              string
}

// NewTranslator returns a new Translator
func NewTranslator(set component.TelemetrySettings, attributesTranslator *attributes.Translator, otelSource string) (*Translator, error) {
	return &Translator{
		set:                  set,
		attributesTranslator: attributesTranslator,
		otelTag:              "otel_source:" + otelSource,
	}, nil
}

func (t *Translator) hostNameAndServiceNameFromResource(ctx context.Context, res pcommon.Resource, hostFromAttributesHandler attributes.HostFromAttributesHandler) (host string, service string) {
	if src, ok := t.attributesTranslator.ResourceToSource(ctx, res, signalTypeSet, hostFromAttributesHandler); ok && src.Kind == source.HostnameKind {
		host = src.Identifier
	}
	if s, ok := res.Attributes().Get(conventions.AttributeServiceName); ok {
		service = s.AsString()
	}
	return host, service
}

func (t *Translator) hostFromAttributes(ctx context.Context, attrs pcommon.Map) string {
	if src, ok := t.attributesTranslator.AttributesToSource(ctx, attrs); ok && src.Kind == source.HostnameKind {
		return src.Identifier
	}
	return ""
}

// MapLogs from OTLP format to Datadog format.
func (t *Translator) MapLogs(ctx context.Context, ld plog.Logs, hostFromAttributesHandler attributes.HostFromAttributesHandler) []datadogV2.HTTPLogItem {
	rsl := ld.ResourceLogs()
	var payloads []datadogV2.HTTPLogItem
	for i := 0; i < rsl.Len(); i++ {
		rl := rsl.At(i)
		sls := rl.ScopeLogs()
		res := rl.Resource()
		host, service := t.hostNameAndServiceNameFromResource(ctx, res, hostFromAttributesHandler)
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			lsl := sl.LogRecords()
			scope := sl.Scope()
			// iterate over Logs
			for k := 0; k < lsl.Len(); k++ {
				log := lsl.At(k)
				// HACK: Check for host and service in log record attributes
				// This is not aligned with the specification and will be removed in the future.
				if host == "" {
					host = t.hostFromAttributes(ctx, log.Attributes())
				}
				if service == "" {
					if s, ok := log.Attributes().Get(conventions.AttributeServiceName); ok {
						service = s.AsString()
					}
				}

				payload := transform(log, host, service, res, scope, t.set.Logger)
				ddtags := payload.GetDdtags()
				if ddtags != "" {
					payload.SetDdtags(ddtags + "," + t.otelTag)
				} else {
					payload.SetDdtags(t.otelTag)
				}
				payloads = append(payloads, payload)
			}
		}
	}
	return payloads
}
