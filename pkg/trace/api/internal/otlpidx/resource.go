// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"context"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// keyStatsComputed is the resource attribute marking stats as computed by the client. It must
// match the key the OTLP receiver reads.
const keyStatsComputed = "_dd.stats_computed"

// Supported reports whether the decoder implements the receiver behaviour selected by conf. It
// covers only the default ReceiveResourceSpansV2 path: callers must fall back to the pdata path
// when a legacy option is set.
func Supported(conf *config.AgentConfig) bool {
	return transform.OperationAndResourceNameV2Enabled(conf) &&
		!conf.HasFeature("disable_receive_resource_spans_v2") &&
		!conf.HasFeature("disable_otlp_container_tags_v2")
}

// pdataResource decodes the serialized Resource message b into a pdata Resource, for the
// attribute helpers that need one. It runs once per payload.
func pdataResource(b []byte) (pcommon.Resource, error) {
	rs := protowire.AppendBytes(protowire.AppendTag(nil, fResourceSpansResource, protowire.BytesType), b)
	req := protowire.AppendBytes(protowire.AppendTag(nil, fExportResourceSpans, protowire.BytesType), rs)
	er := ptraceotlp.NewExportRequest()
	if err := er.UnmarshalProto(req); err != nil {
		return pcommon.Resource{}, err
	}
	return er.Traces().ResourceSpans().At(0).Resource(), nil
}

// resourceSource resolves the payload's source, hostname, container tags and client-computed
// stats from the resource, mirroring OTLPReceiver.receiveResourceSpansV2. Resource attributes
// consumed as container tags are removed from resAttrs, so they aren't also copied to spans.
func (d *decoder) resourceSource(ctx context.Context, resource []byte) error {
	res, err := pdataResource(resource)
	if err != nil {
		return err
	}
	conf := d.opts.Conf
	var src source.Source
	var srcok bool
	if tr := conf.OTLPReceiver.AttributesTranslator; tr != nil {
		src, srcok = tr.ResourceToSource(ctx, res, transform.SignalTypeSet, d.opts.HostFromAttributesHandler)
	}
	if srcok {
		if src.Kind == source.HostnameKind {
			d.res.hostname = src.Identifier //nolint:staticcheck // SA1019: mirrors receiveResourceSpansV2 during the Source.Identifier migration (datadog-agent#51116)
		}
	} else {
		// fallback hostname
		d.res.hostname = conf.Hostname
		src = source.Source{
			Kind:             source.HostnameKind,
			Identifier:       conf.Hostname, //nolint:staticcheck // SA1019: mirrors receiveResourceSpansV2 during the Source.Identifier migration (datadog-agent#51116)
			SourceIdentifier: source.SourceIdentifier{Primary: conf.Hostname},
		}
	}
	d.res.source = src

	if _, ok := res.Attributes().Get(keyStatsComputed); ok {
		s := transform.GetOTelAttrVal(res.Attributes(), true, keyStatsComputed)
		computed := s != "" && s != "false"
		d.res.statsComputed = &computed
	}

	tags, filtered := attributes.ConsumeContainerTagsFromResource(res)
	d.res.containerTags = flattenTags(tags)
	if filtered.Attributes().Len() != res.Attributes().Len() {
		res.Attributes().Range(func(k string, _ pcommon.Value) bool {
			if _, kept := filtered.Attributes().Get(k); !kept {
				delete(d.resAttrs, d.strings.Lookup(k))
			}
			return true
		})
	}
	// Like transform.OtelSpanToDDSpan, resource attributes are copied to spans under their Datadog
	// key and in their string form, whatever their type. Promoted keys set span fields instead
	// (span.kind never does: today's conversion has already set it from the OTel span kind).
	filtered.Attributes().Range(func(k string, v pcommon.Value) bool {
		mapped := transform.GetDDKeyForOTLPAttribute(k)
		if mapped == "" {
			return true
		}
		if bit := promotedKey(mapped); bit != 0 {
			if bit != promotedKind {
				d.res.promoted.put(bit, d.strings.Add(v.AsString()), "")
			}
			return true
		}
		if !d.opts.ResourceAttributesOnPayload {
			d.resMerge = append(d.resMerge, resMerge{
				key: d.strings.Add(mapped),
				val: d.stringValue(d.strings.Add(v.AsString())),
			})
		}
		return true
	})
	return nil
}

// attrAsString returns the value of the serialized KeyValue kv formatted by
// pcommon.Value.AsString, the form today's conversion stores non-numeric span attributes in. It is
// only used for arrays, maps and bytes, so the pdata round trip is acceptable.
func attrAsString(kv []byte) (string, error) {
	res, err := pdataResource(protowire.AppendBytes(protowire.AppendTag(nil, fResourceAttributes, protowire.BytesType), kv))
	if err != nil {
		return "", err
	}
	var s string
	res.Attributes().Range(func(_ string, v pcommon.Value) bool {
		s = v.AsString()
		return false
	})
	return s, nil
}

// flattenTags joins m into a comma-separated list of key:value tags, like the OTLP receiver's
// flatten.
func flattenTags(m map[string]string) string {
	var b strings.Builder
	for k, v := range m {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte(':')
		b.WriteString(v)
	}
	return b.String()
}
