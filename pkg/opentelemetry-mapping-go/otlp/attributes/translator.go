// Copyright  OpenTelemetry Authors
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
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

const missingSourceMetricName string = "datadog.otlp_translator.resources.missing_source"

// Translator of attributes.
type Translator struct {
	missingSources metric.Int64Counter
}

// NewTranslator returns a new attributes translator.
func NewTranslator(set component.TelemetrySettings) (*Translator, error) {
	meter := set.MeterProvider.Meter("github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes")
	missingSources, err := meter.Int64Counter(
		missingSourceMetricName,
		metric.WithDescription("OTLP resources that are missing a source (e.g. hostname)"),
		metric.WithUnit("{resource}"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build missing source counter: %w", err)
	}

	return &Translator{
		missingSources: missingSources,
	}, nil
}

// ResourceToSource gets a telemetry signal source from its resource attributes.
func (p *Translator) ResourceToSource(ctx context.Context, res pcommon.Resource, set attribute.Set, hostFromAttributesHandler HostFromAttributesHandler) (source.Source, bool) {
	src, ok := SourceFromAttrs(res.Attributes(), hostFromAttributesHandler)
	if !ok {
		p.missingSources.Add(ctx, 1, metric.WithAttributeSet(set))
	}

	return src, ok
}

// AttributesToSource gets a telemetry signal source from a set of attributes.
// As opposed to ResourceToSource, this does not keep track of failed requests.
//
// NOTE: This method SHOULD NOT generally be used: it is only used in the logs implementation
// because of a fallback logic that will be removed. The attributes detected are resource attributes,
// not attributes from a telemetry signal.
func (p *Translator) AttributesToSource(_ context.Context, attrs pcommon.Map) (source.Source, bool) {
	return SourceFromAttrs(attrs, nil)
}
