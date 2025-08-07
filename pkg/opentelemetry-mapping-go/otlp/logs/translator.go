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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/otel/attribute"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"
	"go.uber.org/zap"
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
	if s, ok := res.Attributes().Get(string(conventions.ServiceNameKey)); ok {
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
				logRecord := lsl.At(k)
				//update this to actual feature flag
				forward_otlp_rum_to_dd_rum := true
				if forward_otlp_rum_to_dd_rum {
					if _, isRum := logRecord.Attributes().Get("session.id"); isRum {
						client := &http.Client{
							Timeout: 10 * time.Second,
						}

						rattr := rl.Resource().Attributes()
						lattr := logRecord.Attributes()

						// build the Datadog intake URL
						ddforward, _ := rattr.Get("request_ddforward")
						outUrlString := "https://browser-intake-datadoghq.com" +
							ddforward.AsString()

						rumPayload := rum.ConstructRumPayloadFromOTLP(lattr)
						byts, err := json.Marshal(rumPayload)
						if err != nil {
							t.set.Logger.Error("failed to marshal RUM payload: %v", zap.Error(err))
							return []datadogV2.HTTPLogItem{}
						}

						req, err := http.NewRequest("POST", outUrlString, bytes.NewBuffer(byts))
						if err != nil {
							t.set.Logger.Error("failed to create request: %v", zap.Error(err))
							return []datadogV2.HTTPLogItem{}
						}

						// add X-Forwarded-For header containing the request client IP address
						ip, ok := lattr.Get("client.address")
						if ok {
							req.Header.Add("X-Forwarded-For", ip.AsString())
						}

						req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

						// send the request to the Datadog intake URL
						resp, err := client.Do(req)
						if err != nil {
							t.set.Logger.Error("failed to send request: %v", zap.Error(err))
							return []datadogV2.HTTPLogItem{}
						}
						defer func(Body io.ReadCloser) {
							err := Body.Close()
							if err != nil {
							}
						}(resp.Body)

						// read the response body
						body, err := io.ReadAll(resp.Body)
						if err != nil {
							t.set.Logger.Error("failed to read response: %v", zap.Error(err))
							return []datadogV2.HTTPLogItem{}
						}

						// check the status code of the response
						if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
							t.set.Logger.Error("received non-OK response: status: %s, body: %s", zap.String("status", resp.Status), zap.String("body", string(body)))
							return []datadogV2.HTTPLogItem{}
						}
						t.set.Logger.Info("Response:", zap.String("body", string(body)))
						continue
					}
				}
				// HACK: Check for host and service in log record attributes
				// This is not aligned with the specification and will be removed in the future.
				if host == "" {
					host = t.hostFromAttributes(ctx, logRecord.Attributes())
				}
				if service == "" {
					if s, ok := logRecord.Attributes().Get(string(conventions.ServiceNameKey)); ok {
						service = s.AsString()
					}
				}

				payload := transform(logRecord, host, service, res, scope, t.set.Logger)
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
