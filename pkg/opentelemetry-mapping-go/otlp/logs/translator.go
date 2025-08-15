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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
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

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Translator of OTLP logs to Datadog format
type Translator struct {
	set                  component.TelemetrySettings
	attributesTranslator *attributes.Translator
	otelTag              string
	httpClient           HTTPClient
}

// NewTranslator returns a new Translator
func NewTranslator(set component.TelemetrySettings, attributesTranslator *attributes.Translator, otelSource string) (*Translator, error) {
	return &Translator{
		set:                  set,
		attributesTranslator: attributesTranslator,
		otelTag:              "otel_source:" + otelSource,
		httpClient:           nil,
	}, nil
}

func NewTranslatorWithHTTPClient(set component.TelemetrySettings, attributesTranslator *attributes.Translator, otelSource string, client HTTPClient) (*Translator, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Translator{
		set:                  set,
		attributesTranslator: attributesTranslator,
		otelTag:              "otel_source:" + otelSource,
		httpClient:           client,
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

type ParamValue struct {
	ParamKey string
	SpanAttr string
	Fallback string
}

func getParamValue(rattrs pcommon.Map, lattrs pcommon.Map, param ParamValue) string {
	if param.SpanAttr != "" {
		parts := strings.Split(param.SpanAttr, ".")
		m := lattrs
		for i, part := range parts {
			if v, ok := m.Get(part); ok {
				if i == len(parts)-1 {
					return v.AsString()
				}
				if v.Type() == pcommon.ValueTypeMap {
					m = v.Map()
				}
			}
		}
	}
	if v, ok := rattrs.Get(param.ParamKey); ok {
		return v.AsString()
	}
	return param.Fallback
}

func buildDDTags(rattrs pcommon.Map, lattrs pcommon.Map) string {
	requiredTags := []ParamValue{
		{ParamKey: "service", SpanAttr: "service.name", Fallback: "otlpresourcenoservicename"},
		{ParamKey: "version", SpanAttr: "service.version", Fallback: ""},
		{ParamKey: "sdk_version", SpanAttr: "_dd.sdk_version", Fallback: ""},
		{ParamKey: "env", Fallback: "default"},
	}

	tagMap := make(map[string]string)

	if v, ok := rattrs.Get("ddtags"); ok && v.Type() == pcommon.ValueTypeMap {
		v.Map().Range(func(k string, val pcommon.Value) bool {
			tagMap[k] = val.AsString()
			return true
		})
	}

	for _, tag := range requiredTags {
		val := getParamValue(rattrs, lattrs, tag)
		if val != tag.Fallback {
			tagMap[tag.ParamKey] = val
		}
	}

	var tagParts []string
	for k, v := range tagMap {
		tagParts = append(tagParts, k+":"+v)
	}

	return strings.Join(tagParts, ",")
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func buildIntakeUrlPathAndParameters(rattrs pcommon.Map, lattrs pcommon.Map) string {
	var parts []string

	batchTimeParam := ParamValue{ParamKey: "batch_time", Fallback: strconv.FormatInt(time.Now().UnixMilli(), 10)}
	parts = append(parts, batchTimeParam.ParamKey+"="+getParamValue(rattrs, lattrs, batchTimeParam))

	parts = append(parts, "ddtags="+buildDDTags(rattrs, lattrs))

	ddsourceParam := ParamValue{ParamKey: "ddsource", SpanAttr: "source", Fallback: "browser"}
	parts = append(parts, ddsourceParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddsourceParam))

	ddEvpOriginParam := ParamValue{ParamKey: "dd-evp-origin", SpanAttr: "source", Fallback: "browser"}
	parts = append(parts, ddEvpOriginParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddEvpOriginParam))

	ddRequestId, err := randomID()
	if err != nil {
		return ""
	}
	ddRequestIdParam := ParamValue{ParamKey: "dd-request-id", SpanAttr: "", Fallback: ddRequestId}
	parts = append(parts, ddRequestIdParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddRequestIdParam))

	ddApiKeyParam := ParamValue{ParamKey: "dd-api-key", SpanAttr: "", Fallback: ""}
	parts = append(parts, ddApiKeyParam.ParamKey+"="+getParamValue(rattrs, lattrs, ddApiKeyParam))

	return "/api/v2/rum?" + strings.Join(parts, "&")
}

// MapLogsAndRouteRUMEvents from OTLP format to Datadog format if shouldForwardOTLPRUMToDDRUM is true.
func (t *Translator) MapLogsAndRouteRUMEvents(ctx context.Context, ld plog.Logs, hostFromAttributesHandler attributes.HostFromAttributesHandler, shouldForwardOTLPRUMToDDRUM bool, rumIntakeUrl string) ([]datadogV2.HTTPLogItem, error) {
	if t.httpClient == nil {
		return nil, fmt.Errorf("httpClient is nil")
	}

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
				if shouldForwardOTLPRUMToDDRUM {
					if _, isRum := logRecord.Attributes().Get("session.id"); isRum {
						rattr := rl.Resource().Attributes()
						lattr := logRecord.Attributes()

						// build the Datadog intake URL
						pathAndParams := buildIntakeUrlPathAndParameters(rattr, lattr)
						outUrlString := rumIntakeUrl + pathAndParams

						rumPayload := rum.ConstructRumPayloadFromOTLP(lattr)
						byts, err := json.Marshal(rumPayload)
						if err != nil {
							return []datadogV2.HTTPLogItem{}, fmt.Errorf("failed to marshal RUM payload: %w", err)
						}

						req, err := http.NewRequest("POST", outUrlString, bytes.NewBuffer(byts))
						if err != nil {
							return []datadogV2.HTTPLogItem{}, fmt.Errorf("failed to create request: %w", err)
						}

						// add X-Forwarded-For header containing the request client IP address
						ip, ok := lattr.Get("client.address")
						if ok {
							req.Header.Add("X-Forwarded-For", ip.AsString())
						}

						req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

						// send the request to the Datadog intake URL
						resp, err := t.httpClient.Do(req)
						if err != nil {
							return []datadogV2.HTTPLogItem{}, fmt.Errorf("failed to send request: %w", err)
						}
						if resp != nil && resp.Body != nil {
							defer func() {
								if cerr := resp.Body.Close(); cerr != nil {
									t.set.Logger.Error("failed to close response body: %v", zap.Error(cerr))
								}
							}()
						}

						// read the response body
						body, err := io.ReadAll(resp.Body)
						if err != nil {
							return []datadogV2.HTTPLogItem{}, fmt.Errorf("failed to read response: %w", err)
						}

						// check the status code of the response
						if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
							return []datadogV2.HTTPLogItem{}, fmt.Errorf("received non-OK response: status: %s, body: %s", resp.Status, string(body))
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
	return payloads, nil
}

// MapLogs from OTLP format to Datadog format.
// Deprecated: Deprecated in favor of MapLogsAndRouteRUMEvents.
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
					if s, ok := log.Attributes().Get(string(conventions.ServiceNameKey)); ok {
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
