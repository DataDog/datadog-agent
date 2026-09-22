// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transform implements mappings from OTLP to DD semantics, and helpers
package transform

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OperationAndResourceNameV2Enabled checks if the new operation and resource name logic should be used
func OperationAndResourceNameV2Enabled(conf *config.AgentConfig) bool {
	return !conf.OTLPReceiver.SpanNameAsResourceName && len(conf.OTLPReceiver.SpanNameRemappings) == 0 && !conf.HasFeature("disable_operation_and_resource_name_logic_v2")
}

// otelSpanToDDSpanMinimal is the internal implementation shared by OtelSpanToDDSpanMinimal and
// OtelSpanToDDSpan, accepting a pre-created accessor to avoid repeated allocation.
func otelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
	primaryTagKeys []string,
	spanAccessor semantics.Accessor,
) *pb.Span {
	spanKind := otelspan.Kind()

	sattr := otelspan.Attributes()
	rattr := otelres.Attributes()

	ddspan := &pb.Span{
		Service:  GetOTelServiceWithAccessor(spanAccessor, true),
		TraceID:  OTelTraceIDToUint64(otelspan.TraceID()),
		SpanID:   OTelSpanIDToUint64(otelspan.SpanID()),
		ParentID: OTelSpanIDToUint64(otelspan.ParentSpanID()),
		Start:    int64(otelspan.StartTimestamp()),
		Duration: int64(otelspan.EndTimestamp()) - int64(otelspan.StartTimestamp()),
		Meta:     make(map[string]string, sattr.Len()+rattr.Len()),
		Metrics:  make(map[string]float64),
	}

	if otelspan.Status().Code() == ptrace.StatusCodeError {
		ddspan.Error = 1
	}

	if OperationAndResourceNameV2Enabled(conf) {
		ddspan.Name = GetOTelOperationNameV2WithAccessor(otelspan, spanAccessor)
		ddspan.Resource = GetOTelResourceV2WithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Name = GetOTelOperationNameV1(otelspan, otelres, lib, conf.OTLPReceiver.SpanNameAsResourceName, conf.OTLPReceiver.SpanNameRemappings, true)
		ddspan.Resource = GetOTelResourceV1(otelspan, otelres)
	}
	// This path bypasses the trace-agent's normalization step so we explicitly do it here.
	ddspan.Name, _ = normalizeutil.NormalizeName(ddspan.Name)

	// correct span type logic if using new resource receiver, keep same if on v1. separate from OperationAndResourceNameV2Enabled.
	if !conf.HasFeature("disable_receive_resource_spans_v2") {
		ddspan.Type = GetOTelSpanTypeWithAccessor(otelspan, spanAccessor)
	} else {
		ddspan.Type = LookupSemanticStringFromDualMaps(rattr, sattr, semantics.ConceptSpanType, true)
		if ddspan.Type == "" {
			ddspan.Type = SpanKind2Type(otelspan, otelres)
		}
	}

	ddspan.Meta["span.kind"] = OTelSpanKindName(spanKind)

	reg := semantics.DefaultRegistry()
	if code, ok := semantics.LookupInt64(reg, spanAccessor, semantics.ConceptHTTPStatusCode); ok && code >= 0 {
		ddspan.Metrics[traceutil.TagStatusCode] = float64(code)
	}
	if grpcCode := semantics.LookupString(reg, spanAccessor, semantics.ConceptGRPCStatusCode); grpcCode != "" {
		ddspan.Meta[string(semantics.ConceptGRPCStatusCode)] = grpcCode
	}
	if isTopLevel {
		traceutil.SetTopLevel(ddspan, true)
	}
	if v, ok := semantics.LookupInt64(reg, spanAccessor, semantics.ConceptDDMeasured); ok && v == 1 {
		traceutil.SetMeasured(ddspan, true)
	} else if topLevelByKind && (spanKind == ptrace.SpanKindClient || spanKind == ptrace.SpanKindProducer) {
		// When enable_otlp_compute_top_level_by_span_kind is true, compute stats for client-side spans
		traceutil.SetMeasured(ddspan, true)
	}
	// TODO(DSEM-658): migrate peer tag lookups to use the semantics library.
	// Use direct lookup only: peerTagKeys already lists every precursor from peer_tags.ini (semantic-core).
	for _, peerTagKey := range peerTagKeys {
		if peerTagVal := GetOTelAttrFromEitherMap(sattr, rattr, false, peerTagKey); peerTagVal != "" {
			ddspan.Meta[peerTagKey] = peerTagVal
		}
	}
	// This path bypasses the trace-agent's normalization step so we explicitly do it here.
	if pSvc, ok := ddspan.Meta[string(semantics.ConceptPeerService)]; ok {
		ddspan.Meta[string(semantics.ConceptPeerService)], _ = normalizeutil.NormalizePeerService(pSvc)
	}
	if bSvc, ok := ddspan.Meta[string(semantics.ConceptDDBaseService)]; ok {
		ddspan.Meta[string(semantics.ConceptDDBaseService)], _ = normalizeutil.NormalizePeerService(bSvc)
	}
	ddspan.Duration, _ = normalizeutil.FixDuration(ddspan.Start, ddspan.Duration)
	ddspan.Start, _ = normalizeutil.FixStartTime(ddspan.Start, ddspan.Duration)
	// Copy span-derived primary tag values into Meta so the APM stats
	// Concentrator's matchingAdditionalMetricTags (which reads span.Meta[key])
	// can aggregate on them. The minimal conversion does not copy all
	// attributes, so these must be pulled in explicitly like peer tags above.
	for _, primaryTagKey := range primaryTagKeys {
		if primaryTagVal := GetOTelAttrFromEitherMap(sattr, rattr, false, primaryTagKey); primaryTagVal != "" {
			ddspan.Meta[primaryTagKey] = primaryTagVal
		}
	}
	// Resolve the AWS identity tags that stats can aggregate on, for the same
	// reason the two loops above exist: the stats path builds its spans here, so
	// a tag the full conversion derives from OpenTelemetry attributes would
	// otherwise be missing from the stats span and the two would disagree. Only
	// the requested keys are mapped, so this adds nothing a stats span does not
	// aggregate on. Without it an OTLP DynamoDB span resolves
	// peer.aws.dynamodb.table on the trace path (which has tablename) but not on
	// the OTLP stats path, whose only precursor for that concept is tablename, a
	// key no OpenTelemetry producer sets.
	if !conf.HasFeature(DisableAWSIdentityTagsFeature) {
		mapAWSIdentityAttributesForKeys(sattr, rattr, ddspan, peerTagKeys, primaryTagKeys)
	}
	// Preserve the raw W3C tracestate so downstream consumers of the minimal
	// span (e.g. the APM stats Concentrator) can recover head-sampling
	// probability and weight stats accordingly. The full OtelSpanToDDSpan
	// conversion already does this; mirror it here.
	// An explicit _sample_rate attribute set by an upstream tracer takes
	// precedence over the value decoded from the tracestate below. Apply it first
	// so SetSampleRateFromTracestate's "gated on absence" guard preserves it.
	SetSampleRateFromAttribute(ddspan, sattr)
	if ts := otelspan.TraceState().AsRaw(); ts != "" {
		ddspan.Meta["w3c.tracestate"] = ts
		// Decode the head-based sampling probability from the tracestate and set
		// _sample_rate so the APM stats Concentrator scales stats back up by the
		// head-sampling weight (1/_sample_rate). Gated on absence to preserve any
		// explicit upstream value (including the _sample_rate attribute above).
		SetSampleRateFromTracestate(ddspan, ts)
	}
	return ddspan
}

// OtelSpanToDDSpanMinimal converts an OTel span to a DD span with only the minimal fields
// needed for APM stats calculation. Only use with OTLPTracesToConcentratorInputs.
func OtelSpanToDDSpanMinimal(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	isTopLevel, topLevelByKind bool,
	conf *config.AgentConfig,
	peerTagKeys []string,
	primaryTagKeys []string,
) *pb.Span {
	spanAccessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	return otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, peerTagKeys, primaryTagKeys, spanAccessor)
}

func isDatadogAPMConventionKey(k string) bool {
	return k == "service.name" || k == "operation.name" || k == "resource.name" || k == "span.type"
}

// GetDDKeyForOTLPAttribute looks for a key in the Datadog HTTP convention that matches the given key from the
// OTLP HTTP convention. Otherwise, check if it is a Datadog APM convention key - if it is, it will be handled with
// specialized logic elsewhere, so return an empty string. If it isn't, return the original key.
func GetDDKeyForOTLPAttribute(k string) string {
	mappedKey, found := attributes.HTTPMappings[k]
	switch {
	case found:
		break
	case strings.HasPrefix(k, "http.request.header."):
		mappedKey = "http.request.headers." + strings.TrimPrefix(k, "http.request.header.")
	case !isDatadogAPMConventionKey(k):
		mappedKey = k
	default:
		return ""
	}
	return mappedKey
}

func conditionallyMapOTLPAttributeToMeta(k string, value string, ddspan *pb.Span) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if ddspan.Meta[mappedKey] != "" {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled explicitly elsewhere.
	if mappedKey != "" {
		SetMetaOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

// MapAWSIdentityAttributes adds the Datadog tag names for AWS resource identity
// carried by OpenTelemetry attributes (see attributes.AWSIdentitySpanMappings).
// The OpenTelemetry keys are kept; the Datadog names are added alongside them
// and never overwrite a tag the producer set itself. Span attributes take
// precedence over resource attributes, matching the Meta copy loops.
//
// The caller must invoke this after every loop that copies attributes into Meta:
// the span and resource attribute loops, and also the instrumentation-scope
// attribute copy where the caller performs one. The scope copy assigns into Meta
// unconditionally, so running before it would let a scope attribute named
// region or aws_account replace the identity resolved from the span or resource.
// Running last keeps that from happening while still leaving a scope attribute
// in place when there is no AWS identity to map; see setAWSIdentityMeta.
//
// cloud.region is mapped to region unconditionally, because region is a
// provider-neutral Datadog tag (ContainerMappings maps it the same way for
// every provider). Only cloud.account.id is provider-gated, because aws_account
// is AWS-specific; see awsAccountID.
//
// Note on aws.lambda.invoked_arn: the OpenTelemetry attribute carries the ARN as
// invoked, which may be qualified with a version or an alias
// (...:function:name:1, ...:function:name:live), whereas the Datadog serverless
// instrumentation generally reports function_arn unqualified. The value is
// mapped as received, so for a qualified invocation the resulting function_arn
// does NOT match the native tag for the same function, and that span will not
// group with natively instrumented ones. Stripping the qualifier would discard
// which version or alias was actually invoked, so the qualifier is kept and the
// mismatch is accepted; unqualified invocations, which are the common case,
// match natively instrumented spans exactly.
func MapAWSIdentityAttributes(sattr, rattr pcommon.Map, ddspan *pb.Span) {
	mapAWSIdentityAttributes(sattr, rattr, ddspan, awsIdentityAllTags)
}

// DisableAWSIdentityTagsFeature turns the AWS identity tag mapping off, for
// operators who need the previous span tags and APM stats aggregation keys back
// without downgrading the Agent. It follows the convention of the other
// conversion switches in this package (disable_receive_resource_spans_v2,
// disable_otlp_container_tags_v2, disable_otel_scope_convention): the mapping is
// on by default and the feature disables it on every path that applies it.
//
// It is exported so the receiver's legacy conversion gates on the same constant
// rather than repeating the string. MapAWSIdentityAttributes itself does not
// read it, because it has no config to read; every caller inside this repository
// gates on this feature before calling it.
const DisableAWSIdentityTagsFeature = "disable_otlp_aws_identity_tags"

// awsIdentityTagBits assigns one bit to each Datadog tag name this mapping can
// produce, so that a caller restricting the mapping to a set of tag names is
// resolved in a single pass over its keys rather than a lookup per tag. Reading
// a name that is not a mapping target yields 0, which selects nothing.
//
// It covers the table's targets plus the two tags resolved outside the table
// (aws_account and tablename, which need the provider gate and the single-table
// rule respectively).
var awsIdentityTagBits, awsIdentityAllTags = func() (map[string]uint, uint) {
	bits := make(map[string]uint, len(attributes.AWSIdentitySpanMappings)+2)
	var all uint
	add := func(ddKey string) {
		if _, ok := bits[ddKey]; ok {
			return
		}
		if len(bits) >= 32 {
			// A uint is 32 bits on a 32-bit build, so the shift below would wrap
			// and silently give this tag bit 0, making it unmappable on every
			// path with no build or test failure. Fail loudly instead; the
			// mapping is nowhere near this many tags.
			panic("transform: too many AWS identity tag names for the bitmask")
		}
		bit := uint(1) << len(bits)
		bits[ddKey] = bit
		all |= bit
	}
	for _, ddKey := range attributes.AWSIdentitySpanMappings {
		add(ddKey)
	}
	add("aws_account")
	add("tablename")
	return bits, all
}()

// mapAWSIdentityAttributesForKeys applies the mapping for the Datadog tag names
// in keySets only. The minimal conversion used by the APM stats path copies just
// the attributes stats aggregates on, so it must not gain every identity tag;
// it needs exactly the ones that can take part in aggregation (peer tags and
// additional metric tags) so that a stats span and the corresponding trace span
// agree on those keys. Passing no keys maps nothing.
func mapAWSIdentityAttributesForKeys(sattr, rattr pcommon.Map, ddspan *pb.Span, keySets ...[]string) {
	// One pass over the caller's keys rather than a lookup per identity tag:
	// peerTagKeys is the whole precursor list from the semantics registry when
	// peer tag aggregation is on, so it is long, and this runs per span on the
	// stats path. Only the handful of names this mapping can produce are
	// collected, and the common case is that none of them appears.
	var wantedTags uint
	for _, keys := range keySets {
		for _, k := range keys {
			if bit, ok := awsIdentityTagBits[k]; ok {
				wantedTags |= bit
			}
		}
	}
	if wantedTags == 0 {
		return
	}
	mapAWSIdentityAttributes(sattr, rattr, ddspan, wantedTags)
}

// mapAWSIdentityAttributes is the shared implementation. A wantedTags of
// awsIdentityAllTags maps every tag this mapping can produce, which is the
// table's targets plus aws_account and tablename; any other value restricts the
// mapping to the tags whose bits are set.
func mapAWSIdentityAttributes(sattr, rattr pcommon.Map, ddspan *pb.Span, wantedTags uint) {
	for otelKey, ddKey := range attributes.AWSIdentitySpanMappings {
		if wantedTags&awsIdentityTagBits[ddKey] == 0 {
			continue
		}
		if v := identitySourceValue(sattr, rattr, otelKey); v != "" {
			setAWSIdentityMeta(sattr, rattr, ddspan, ddKey, v)
		}
	}

	// cloud.account.id is only an AWS account ID when the provider is AWS. Other
	// providers use different Datadog tags (subscription_id, project_id) which are
	// out of scope here.
	if wantedTags&awsIdentityTagBits["aws_account"] != 0 {
		if account := awsAccountID(sattr, rattr); account != "" {
			setAWSIdentityMeta(sattr, rattr, ddspan, "aws_account", account)
		}
	}

	if wantedTags&awsIdentityTagBits["tablename"] != 0 {
		// Read from the span scope only, unlike every other mapped key.
		// aws.dynamodb.table_names describes the tables one call targeted, not
		// the resource the process runs as, so a resource-scoped value does not
		// describe any particular span: borrowing it would give every sibling
		// span on that resource, including spans that touch no table at all, a
		// tablename tag and with it the peer.aws.dynamodb.table aggregation key
		// for a table they never touched. A span-only lookup also keeps the two
		// receiver paths and the stats conversion identical, since none of them
		// can reach a different scope.
		if name := singleDynamoDBTableName(sattr); name != "" {
			setAWSIdentityMeta(sattr, rattr, ddspan, "tablename", name)
		}
	}
}

// identitySourceValue returns the value of an OpenTelemetry source key, span
// scope first, matching the precedence of the Meta copy loops.
//
// A numeric span attribute is skipped, because both receiver paths route numeric
// span attributes to Metrics and never to Meta. Reading one here would make the
// mapped Datadog tag come from a scope whose value is not the one the span's
// own OpenTelemetry key carries, which is exactly the skew the mapping exists to
// avoid: the two tags are documented as carrying the same identity.
//
// One residual case remains, in the legacy V1 conversion only: its span
// attribute loop assigns unconditionally, so an explicitly empty span-scope
// value leaves Meta's OpenTelemetry key empty while this lookup falls through to
// the resource. The mapped tag then carries the resource's identity beside an
// empty OpenTelemetry key, which is the better of the two outcomes, since the
// alternative is to map nothing at all.
//
// Note also that span scope wins even for attributes the semantic conventions
// define as resource-scoped (cloud.region, cloud.account.id, aws.ecs.*), and for
// aws.lambda.invoked_arn, which they define on the invoked span. A span
// annotated with a callee's or another account's value therefore yields a
// Datadog identity tag describing that other resource, which can disagree with
// the chunk's container tags derived from the resource. That is deliberate: the
// mapped tag always equals the OpenTelemetry key already present in Meta for the
// same span, and keeping the two in agreement is what prevents aws_account and
// cloud.account.id from naming different accounts on one span. Preferring the
// resource instead would make the Datadog tag contradict its own source key.
func identitySourceValue(sattr, rattr pcommon.Map, otelKey string) string {
	if v, ok := metaBoundValue(sattr, otelKey); ok && v != "" {
		return v
	}
	// Get returns a zero Value when the key is absent, which cannot be read, so
	// the presence result must be checked before using it.
	if v, ok := rattr.Get(otelKey); ok {
		return v.AsString()
	}
	return ""
}

// metaBoundValue returns the value attrs carries under key as the span attribute
// loop would copy it into Meta, and reports whether that value reaches Meta at
// all. A numeric value does not: it is routed to Metrics instead. Callers that
// only need a usable value can ignore the bool and test the string.
func metaBoundValue(attrs pcommon.Map, key string) (string, bool) {
	val, ok := attrs.Get(key)
	if !ok {
		return "", false
	}
	switch val.Type() {
	case pcommon.ValueTypeDouble, pcommon.ValueTypeInt:
		return "", false
	default:
		return val.AsString(), true
	}
}

// singleDynamoDBTableName returns the one table named by aws.dynamodb.table_names
// in attrs, or "" when attrs names no single table.
//
// The Datadog tracers tag a single table as tablename (the
// peer.aws.dynamodb.table precursor), so only a single table is mapped: a
// multi-table operation has no single target, and collapsing it to one of its
// tables would report the wrong peer. The attribute is specified as an array but
// is accepted as a plain string too, which some instrumentations send; a string
// that is itself a rendered list ("a,b", "[a b]", `["a","b"]`) names more than
// one table, or wraps one in syntax that is not part of its name, and is
// rejected for the same reason.
func singleDynamoDBTableName(attrs pcommon.Map) string {
	tableNames, ok := attrs.Get(string(semconv127.AWSDynamoDBTableNamesKey))
	if !ok {
		return ""
	}
	switch tableNames.Type() {
	case pcommon.ValueTypeSlice:
		if s := tableNames.Slice(); s.Len() == 1 {
			return oneTableName(s.At(0).AsString())
		}
		return ""
	case pcommon.ValueTypeStr:
		return oneTableName(tableNames.Str())
	default:
		// A type that cannot name a table: a number, a bool, a map.
		return ""
	}
}

// oneTableName returns s when it names a single DynamoDB table, or "" when it
// does not.
//
// A DynamoDB table name is 3 to 255 characters of letters, digits, underscore,
// hyphen and dot, so anything outside that cannot be one. Checking the charset
// rather than a list of separators rejects every rendering of a multi-table
// array in one pass, whichever separator the instrumentation used ("a,b",
// "a b", "[a b]", `["a","b"]`), and also rejects a value that is not a name at
// all, such as the stringified number a one-element numeric array yields. The
// check applies to a single-element array as well as to a plain string, since an
// instrumentation that renders the whole array into one element names no single
// table either.
//
// A table ARN is therefore not accepted, even though the DynamoDB API itself
// takes one in place of a name: widening the charset to admit an ARN would also
// readmit the rendered-list forms this check exists to reject, and an ARN would
// not match the bare table name a natively instrumented span carries, which is
// the whole point of the mapping. Such a span simply gets no tablename.
func oneTableName(s string) string {
	if len(s) < 3 || len(s) > 255 {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return ""
		}
	}
	return s
}

// setAWSIdentityMeta writes a Datadog AWS identity tag unless the producer set
// that exact tag name itself, as a span or resource attribute.
//
// The guard reads the producer's attribute maps rather than ddspan.Meta so that
// the caller can run after the instrumentation-scope attributes have been copied
// (see MapAWSIdentityAttributes). A scope attribute lands in Meta through an
// unconditional assignment, and reading Meta could not tell such a value apart
// from a producer tag, so a scope key would silently suppress the mapping and
// keep the identity the span or resource actually carries off the span.
//
// Checking the span attributes is equivalent to the Meta check for every value a
// producer can set there, because the span attribute loop is the only step that
// copies these tag names into Meta verbatim: none of them is in
// attributes.HTTPMappings or is a Datadog APM convention key (so
// GetDDKeyForOTLPAttribute returns them unchanged). The minimal conversion's
// peer-tag and primary-tag loops can write one of these names, but they read it
// from these same maps under the same name, so the guard reaches the same
// decision there as it does on the full conversion.
//
// The resource side is deliberately narrower; see producerSetResourceTag.
func setAWSIdentityMeta(sattr, rattr pcommon.Map, ddspan *pb.Span, k, v string) {
	if producerSetSpanTag(sattr, k) || producerSetResourceTag(rattr, k) {
		return
	}
	if ddspan.Meta == nil {
		ddspan.Meta = make(map[string]string)
	}
	ddspan.Meta[k] = v
}

// producerSetSpanTag reports whether the span attributes carry a producer-set
// Datadog tag named k, that is, a value that the span attribute loop copies into
// Meta. Numeric span attributes do not qualify: both receiver paths route them to
// Metrics rather than Meta, so a numeric attribute under a Datadog tag name
// leaves that tag unset and must not suppress the mapping (an account id, for
// instance, can plausibly be sent as an integer). This is the same routing rule
// the source reader applies, so the guard and the reader cannot diverge.
func producerSetSpanTag(sattr pcommon.Map, k string) bool {
	v, _ := metaBoundValue(sattr, k)
	return v != ""
}

// producerSetResourceTag reports whether the resource attributes carry a
// producer-set Datadog tag named k that this conversion can rely on seeing.
//
// A resource attribute whose name is already a Datadog container tag name is
// excluded, because the two receiver paths do not agree on whether it is still
// there by the time the conversion runs. On its default configuration
// receiveResourceSpansV2 calls attributes.ConsumeContainerTagsFromResource
// first, which moves every string-valued resource attribute named after a
// Datadog container tag into the chunk's container tags and REMOVES it from the
// resource, precisely so the conversion does not also emit it as a span tag.
// receiveResourceSpansV1 does no such filtering and passes the resource through
// untouched, as does V2 when disable_otlp_container_tags_v2 is set. Of the
// mapped tag names, region and task_arn are container tag names (they are
// ContainerMappings values), so consulting them here would suppress the mapping
// on the paths that still see them while the default V2 conversion could not,
// and the same payload would produce different span tags depending on the path
// and that feature flag. Ignoring them is uniform across all of them.
//
// Excluding them therefore trades one behavior for parity, and the trade is
// deliberate: when a span carries both the OpenTelemetry key and a bare Datadog
// name for the same identity (cloud.region and region, say), the mapped value
// wins and the bare value is dropped from Meta. That matches what the default V2
// path already does one layer up, where ConsumeContainerTagsFromResource ranks
// an OpenTelemetry-derived container tag ABOVE a bare Datadog-named one, so the
// container tag it emits for region already carries cloud.region's value and the
// bare value is already discarded. Keeping Meta consistent with that precedence
// is the behavior being chosen here. Note the residual case this does not
// change: when only the bare name is set and the OpenTelemetry key is absent,
// nothing is mapped, and the unfiltered paths keep the bare value in Meta while
// the default V2 path has consumed it into container tags. That difference is
// pre-existing container-tag behavior, not something this mapping introduces.
//
// Every other type is accepted, unlike the span check: the resource attribute
// loop copies every type into Meta as a string, so any type counts as set.
//
// An empty value is not treated as set in either check: both receiver paths do
// store an empty attribute in Meta as "", so an empty producer tag is present
// but carries no identity, and writing the resolved identity over it is the
// intended outcome.
func producerSetResourceTag(rattr pcommon.Map, k string) bool {
	if attributes.IsContainerTagName(k) {
		return false
	}
	val, ok := rattr.Get(k)
	return ok && val.AsString() != ""
}

// awsAccountID returns the AWS account ID to alias to aws_account, or "" when
// there is none to alias.
//
// The OpenTelemetry key cloud.account.id is copied to Meta verbatim by the
// attribute loops with span-over-resource precedence, so aws_account must be
// resolved from the SAME scope that supplies that value, or the two tags (which
// are documented as aliases) could carry different identities on one span. The
// scope that owns cloud.account.id is therefore selected first (span, then
// resource), and the alias is produced only when it is proven to be AWS:
//
//   - If that scope declares cloud.provider, it must be aws. A non-AWS provider
//     on the owning scope means the id is not an AWS account (it is a GCP project,
//     an Azure subscription, ...), so no alias is produced.
//   - If the owning scope declares no provider of its own, the other scope's
//     provider may supply it, but only when no scope declares a non-aws provider.
//     This recovers the conventional split layout (cloud.provider=aws on the
//     resource, cloud.account.id on the span) without letting a provider from one
//     scope authorize an account id that a conflicting provider elsewhere denies.
//
// The account id is never read from a different scope than the provider that is
// allowed to license it, so a non-AWS account can never be mislabeled aws_account.
func awsAccountID(sattr, rattr pcommon.Map) string {
	account, accountFromSpan, ok := accountIDWithScope(sattr, rattr)
	if !ok || account == "" {
		return ""
	}
	accountScope := rattr
	if accountFromSpan {
		accountScope = sattr
	}

	// The scope that owns the account id gates it when it declares a provider.
	// An empty cloud.provider is treated as absent, not as a conflicting
	// provider: the attribute loops skip an empty value, so it asserts nothing
	// about the cloud the account belongs to.
	if provider := cloudProvider(accountScope); provider != "" {
		if provider == semconv127.CloudProviderAWS.Value.AsString() {
			return account
		}
		return ""
	}

	// The owning scope is provider-less: the account is AWS only if some scope
	// says aws and none says anything else.
	var sawAWS, sawOther bool
	for _, m := range []pcommon.Map{sattr, rattr} {
		if provider := cloudProvider(m); provider != "" {
			if provider == semconv127.CloudProviderAWS.Value.AsString() {
				sawAWS = true
			} else {
				sawOther = true
			}
		}
	}
	if sawAWS && !sawOther {
		return account
	}
	return ""
}

// cloudProvider returns the cloud.provider declared by attrs, or "" when attrs
// declares none.
//
// An attribute present with an empty value reports "", so that an empty
// cloud.provider neither authorizes nor denies an account id. A numeric value
// also reports "": it cannot equal the conventional provider string, so it
// declares no provider. The comparison against the convention's value is exact,
// as everywhere else in this package: cloud.provider is a closed enumeration
// whose members are lower case, so a differently cased value is not the
// convention's aws.
func cloudProvider(attrs pcommon.Map) string {
	provider, _ := metaBoundValue(attrs, string(semconv127.CloudProviderKey))
	return provider
}

// accountIDWithScope returns cloud.account.id and whether it came from the span
// scope, trying the span scope before the resource scope to match the precedence
// of the Meta copy loop.
//
// A value that the span attribute loop would not copy into Meta is skipped, so
// the scope selected here is the scope that supplies Meta's cloud.account.id: a
// numeric span value is routed to Metrics instead. An empty value is skipped
// too; both receiver paths do store it in Meta as "", but it carries no account
// to gate or to map, so it must not shadow a real account id in the other scope.
// The bool result is false when neither scope carries a usable value.
func accountIDWithScope(sattr, rattr pcommon.Map) (account string, fromSpan bool, ok bool) {
	if v, _ := metaBoundValue(sattr, string(semconv127.CloudAccountIDKey)); v != "" {
		return v, true, true
	}
	if v, present := rattr.Get(string(semconv127.CloudAccountIDKey)); present && v.AsString() != "" {
		return v.AsString(), false, true
	}
	return "", false, false
}

func conditionallyMapOTLPAttributeToMetric(k string, value float64, ddspan *pb.Span) {
	mappedKey := GetDDKeyForOTLPAttribute(k)
	if _, ok := ddspan.Metrics[mappedKey]; ok {
		return
	}

	// Exclude Datadog APM conventions.
	// These are handled explicitly elsewhere.
	if mappedKey != "" {
		SetMetricOTLPIfEmpty(ddspan, mappedKey, value)
	}
}

// GetOTelEnv returns the environment based on OTel span and resource attributes, with span taking precedence.
func GetOTelEnv(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptDeploymentEnv, true)
}

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes, with span taking precedence.
func GetOTelHostname(span ptrace.Span, res pcommon.Resource, tr *attributes.Translator, fallbackHost string) string {
	ctx := context.Background()
	src, srcok := tr.ResourceToSource(ctx, res, SignalTypeSet, nil)
	if !srcok {
		if v := GetOTelAttrValInResAndSpanAttrs(span, res, false, "_dd.hostname"); v != "" {
			src = source.Source{
				Kind:             source.HostnameKind,
				Identifier:       v, //nolint:staticcheck // SA1019: intentional during Step 1 of the Source.Identifier migration (datadog-agent#51116); this call site migrates to SourceIdentifier.Primary in Step 2
				SourceIdentifier: source.SourceIdentifier{Primary: v},
			}
			srcok = true
		}
	}
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			return src.Identifier //nolint:staticcheck // SA1019: intentional during Step 1 of the Source.Identifier migration (datadog-agent#51116); this call site migrates to SourceIdentifier.Primary in Step 2
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			return ""
		}
	}
	// fallback hostname from Agent conf.Hostname
	return fallbackHost
}

// GetOTelVersion returns the version based on OTel span and resource attributes, with span taking precedence.
func GetOTelVersion(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptServiceVersion, true)
}

// GetOTelContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
func GetOTelContainerID(span ptrace.Span, res pcommon.Resource) string {
	return LookupSemanticStringFromDualMaps(span.Attributes(), res.Attributes(), semantics.ConceptContainerID, true)
}

// GetOTelContainerOrPodID returns the container ID based on OTel span and resource attributes, with span taking precedence.
//
// The Kubernetes pod UID will be used as a fallback if the container ID is not found.
// This is only done for backward compatibility; consider using GetOTelContainerID instead.
func GetOTelContainerOrPodID(span ptrace.Span, res pcommon.Resource) string {
	accessor := semantics.NewOTelSpanAccessor(span.Attributes(), res.Attributes())
	if id := LookupSemanticStringWithAccessor(accessor, semantics.ConceptContainerID, true); id != "" {
		return id
	}
	return LookupSemanticStringWithAccessor(accessor, semantics.ConceptK8sPodUID, true)
}

// GetOTelStatusCode returns the HTTP status code based on OTel span and resource attributes, with span taking precedence.
func GetOTelStatusCode(span ptrace.Span, res pcommon.Resource) uint32 {
	code, ok := LookupSemanticInt64(span.Attributes(), res.Attributes(), semantics.ConceptHTTPStatusCode)
	if !ok || code < 0 {
		return 0
	}
	return uint32(code)
}

// GetOTelContainerTags returns a list of DD container tags in the OTel resource attributes.
// Tags are always normalized.
func GetOTelContainerTags(rattrs pcommon.Map, tagKeys []string) []string {
	var containerTags []string
	containerTagsMap := attributes.ContainerTagsFromResourceAttributes(rattrs)
	for _, key := range tagKeys {
		if mappedKey, ok := attributes.ContainerMappings[key]; ok {
			// If the key has a mapping in ContainerMappings, use the mapped key
			if val, ok := containerTagsMap[mappedKey]; ok {
				t := normalizeutil.NormalizeTag(mappedKey + ":" + val)
				containerTags = append(containerTags, t)
			}
		} else {
			// Otherwise populate as additional container tags
			if val := GetOTelAttrVal(rattrs, false, key); val != "" {
				t := normalizeutil.NormalizeTag(key + ":" + val)
				containerTags = append(containerTags, t)
			}
		}
	}
	return containerTags
}

// OtelSpanToDDSpan converts an OTel span to a DD span.
func OtelSpanToDDSpan(
	otelspan ptrace.Span,
	otelres pcommon.Resource,
	lib pcommon.InstrumentationScope,
	conf *config.AgentConfig,
) *pb.Span {
	spanKind := otelspan.Kind()
	topLevelByKind := conf.HasFeature("enable_otlp_compute_top_level_by_span_kind")
	isTopLevel := false
	if topLevelByKind {
		isTopLevel = otelspan.ParentSpanID() == pcommon.NewSpanIDEmpty() || spanKind == ptrace.SpanKindServer || spanKind == ptrace.SpanKindConsumer
	}
	// Create one shared accessor for all span+resource lookups in this function and in the
	// minimal span conversion below, avoiding repeated allocation of accessor objects.
	spanAccessor := semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes())
	// primaryTagKeys is nil here: the full conversion below copies all span and
	// resource attributes into Meta, so span-derived primary tags are already present.
	ddspan := otelSpanToDDSpanMinimal(otelspan, otelres, lib, isTopLevel, topLevelByKind, conf, nil, nil, spanAccessor)

	// Span attributes take precedence over resource attributes in the event of key collisions; so, use span attributes first
	otelspan.Attributes().Range(func(k string, v pcommon.Value) bool {
		switch v.Type() {
		case pcommon.ValueTypeDouble:
			conditionallyMapOTLPAttributeToMetric(k, v.Double(), ddspan)
		case pcommon.ValueTypeInt:
			conditionallyMapOTLPAttributeToMetric(k, float64(v.Int()), ddspan)
		default:
			conditionallyMapOTLPAttributeToMeta(k, v.AsString(), ddspan)
		}

		return true
	})

	traceID := otelspan.TraceID()
	ddspan.Meta["otel.trace_id"] = hex.EncodeToString(traceID[:])
	if !spanMetaHasKey(ddspan, "version") {
		if version := LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptServiceVersion, true); version != "" {
			ddspan.Meta["version"] = version
		}
	}

	if otelspan.Events().Len() > 0 {
		ddspan.Meta["events"] = MarshalEvents(otelspan.Events())
	}
	TagSpanIfContainsExceptionEvent(otelspan, ddspan)
	if otelspan.Links().Len() > 0 {
		ddspan.Meta["_dd.span_links"] = MarshalLinks(otelspan.Links())
	}

	// Note: w3c.tracestate is set by otelSpanToDDSpanMinimal above.
	scopeConventionGateEnabled := !conf.HasFeature("disable_otel_scope_convention")
	if lib.Name() != "" {
		if scopeConventionGateEnabled {
			ddspan.Meta[string(semconv.OtelScopeNameKey)] = lib.Name()
		}
		// otel.library.name is a deprecated alias of otel.scope.name but MUST still be
		// reported with the same value for backward compatibility.
		ddspan.Meta[string(semconv.OtelLibraryNameKey)] = lib.Name()
	}
	if lib.Version() != "" {
		if scopeConventionGateEnabled {
			ddspan.Meta[string(semconv.OtelScopeVersionKey)] = lib.Version()
		}
		ddspan.Meta[string(semconv.OtelLibraryVersionKey)] = lib.Version()
	}
	ddspan.Meta[string(semconv.OtelStatusCodeKey)] = otelspan.Status().Code().String()
	if msg := otelspan.Status().Message(); msg != "" {
		ddspan.Meta[string(semconv.OtelStatusDescriptionKey)] = msg
	}

	if !spanMetaHasKey(ddspan, "error.msg") || !spanMetaHasKey(ddspan, "error.type") || !spanMetaHasKey(ddspan, "error.stack") {
		ddspan.Error = Status2Error(otelspan.Status(), otelspan.Events(), ddspan.Meta)
	}

	if !spanMetaHasKey(ddspan, "env") {
		if env := LookupSemanticStringWithAccessor(spanAccessor, semantics.ConceptDeploymentEnv, true); env != "" {
			ddspan.Meta["env"] = env
		}
	}

	otelres.Attributes().Range(func(k string, v pcommon.Value) bool {
		value := v.AsString()
		conditionallyMapOTLPAttributeToMeta(k, value, ddspan)
		return true
	})

	for k, v := range lib.Attributes().Range {
		ddspan.Meta[k] = v.AsString()
	}

	// Runs after the scope attribute copy above, which assigns into Meta
	// unconditionally: a scope attribute named region or aws_account must not
	// replace the AWS identity the span or resource carries. A scope attribute
	// still supplies the tag when there is no AWS identity to map.
	if !conf.HasFeature(DisableAWSIdentityTagsFeature) {
		MapAWSIdentityAttributes(otelspan.Attributes(), otelres.Attributes(), ddspan)
	}

	// Check for db.namespace and conditionally set db.name.
	// db.namespace is a resource-level attribute; check resource attrs first, then span attrs.
	if _, ok := ddspan.Meta["db.name"]; !ok {
		if dbNamespace := LookupSemanticStringWithAccessor(
			semantics.NewOTelSpanAccessor(otelspan.Attributes(), otelres.Attributes()),
			semantics.ConceptDBNamespace, false,
		); dbNamespace != "" {
			ddspan.Meta["db.name"] = dbNamespace
		}
	}

	return ddspan
}

// TagSpanIfContainsExceptionEvent tags spans that contain at least on exception span event.
func TagSpanIfContainsExceptionEvent(otelspan ptrace.Span, ddspan *pb.Span) {
	for i := range otelspan.Events().Len() {
		if otelspan.Events().At(i).Name() == "exception" {
			ddspan.Meta["_dd.span_events.has_exception"] = "true"
			return
		}
	}
}

// MarshalEvents marshals events into JSON.
func MarshalEvents(events ptrace.SpanEventSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < events.Len(); i++ {
		e := events.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		var wrote bool
		str.WriteString("{")
		if v := e.Timestamp(); v != 0 {
			str.WriteString(`"time_unix_nano":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
			wrote = true
		}
		if v := e.Name(); v != "" {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"name":`)
			if name, err := json.Marshal(v); err == nil {
				str.WriteString(string(name))
			} else {
				// still collect the event information, if possible
				log.Errorf("Error parsing span event name %v, using name 'redacted' instead", name)
				str.WriteString(`"redacted"`)
			}
			wrote = true
		}
		if e.Attributes().Len() > 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"attributes":{`)
			j := 0
			e.Attributes().Range(func(k string, v pcommon.Value) bool {
				// collect the attribute only if the key is json-parseable, else drop the attribute
				if key, err := json.Marshal(k); err == nil {
					if j > 0 {
						str.WriteString(",")
					}
					str.WriteString(string(key))
					str.WriteString(":")
					if val, err := json.Marshal(v.AsRaw()); err == nil {
						str.WriteString(string(val))
					} else {
						log.Warnf("Trouble parsing the following attribute value, dropping: %v", v.AsString())
						str.WriteString(`"redacted"`)
					}
					j++
				} else {
					log.Errorf("Error parsing the following attribute key on span event %v, dropping attribute: %v", e.Name(), k)
					e.SetDroppedAttributesCount(e.DroppedAttributesCount() + 1)
				}
				j++
				return true
			})
			str.WriteString("}")
			wrote = true
		}
		if v := e.DroppedAttributesCount(); v != 0 {
			if wrote {
				str.WriteString(",")
			}
			str.WriteString(`"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(v), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// MarshalLinks marshals span links into JSON.
func MarshalLinks(links ptrace.SpanLinkSlice) string {
	var str strings.Builder
	str.WriteString("[")
	for i := 0; i < links.Len(); i++ {
		l := links.At(i)
		if i > 0 {
			str.WriteString(",")
		}
		t := l.TraceID()
		str.WriteString(`{"trace_id":"`)
		str.WriteString(hex.EncodeToString(t[:]))
		s := l.SpanID()
		str.WriteString(`","span_id":"`)
		str.WriteString(hex.EncodeToString(s[:]))
		str.WriteString(`"`)
		if ts := l.TraceState().AsRaw(); len(ts) > 0 {
			str.WriteString(`,"tracestate":"`)
			str.WriteString(ts)
			str.WriteString(`"`)
		}
		if l.Attributes().Len() > 0 {
			str.WriteString(`,"attributes":{`)
			var b bool
			l.Attributes().Range(func(k string, v pcommon.Value) bool {
				if b {
					str.WriteString(",")
				}
				b = true
				str.WriteString(`"`)
				str.WriteString(k)
				str.WriteString(`":"`)
				str.WriteString(v.AsString())
				str.WriteString(`"`)
				return true
			})
			str.WriteString("}")
		}
		if l.DroppedAttributesCount() > 0 {
			str.WriteString(`,"dropped_attributes_count":`)
			str.WriteString(strconv.FormatUint(uint64(l.DroppedAttributesCount()), 10))
		}
		str.WriteString("}")
	}
	str.WriteString("]")
	return str.String()
}

// SetMetaOTLP sets the k/v OTLP attribute pair as a tag on span s.
func SetMetaOTLP(s *pb.Span, k, v string) {
	switch k {
	case "operation.name":
		s.Name = v
	case "service.name":
		s.Service = v
	case "resource.name":
		s.Resource = v
	case "span.type":
		s.Type = v
	case "analytics.event":
		if v, err := strconv.ParseBool(v); err == nil {
			if v {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 1
			} else {
				s.Metrics[sampler.KeySamplingRateEventExtraction] = 0
			}
		}
	default:
		s.Meta[k] = v
	}
}

// SetMetaOTLPIfEmpty sets the k/v OTLP attribute pair as a tag on span s, if the corresponding value hasn't been set already.
func SetMetaOTLPIfEmpty(s *pb.Span, k, v string) {
	switch k {
	case "operation.name":
		if s.Name == "" {
			s.Name = v
		}
	case "service.name":
		if s.Service == "" {
			s.Service = v
		}
	case "resource.name":
		if s.Resource == "" {
			s.Resource = v
		}
	case "span.type":
		if s.Type == "" {
			s.Type = v
		}
	case "analytics.event":
		if v, err := strconv.ParseBool(v); err == nil {
			if _, ok := s.Metrics[sampler.KeySamplingRateEventExtraction]; !ok {
				if v {
					s.Metrics[sampler.KeySamplingRateEventExtraction] = 1
				} else {
					s.Metrics[sampler.KeySamplingRateEventExtraction] = 0
				}
			}
		}
	default:
		s.Meta[k] = v
	}
}

// SetMetricOTLP sets the k/v OTLP attribute pair as a metric on span s.
func SetMetricOTLP(s *pb.Span, k string, v float64) {
	switch k {
	case "sampling.priority":
		s.Metrics["_sampling_priority_v1"] = v
	default:
		s.Metrics[k] = v
	}
}

// SetMetricOTLPIfEmpty sets the k/v OTLP attribute pair as a metric on span s, if the corresponding value hasn't been set already.
func SetMetricOTLPIfEmpty(s *pb.Span, k string, v float64) {
	var key string
	switch k {
	case "sampling.priority":
		key = "_sampling_priority_v1"
	default:
		key = k
	}
	if _, ok := s.Metrics[key]; !ok {
		s.Metrics[key] = v
	}
}

// Status2Error checks the given status and events and applies any potential error and messages
// to the given span attributes.
// Note: This function uses the semantic registry for http.status_code lookup precedence.
// Previously it used NEW-first precedence (http.response.status_code before http.status_code).
func Status2Error(status ptrace.Status, _ ptrace.SpanEventSlice, metaMap map[string]string) int32 {
	if status.Code() != ptrace.StatusCodeError {
		return 0
	}
	if _, ok := metaMap["error.msg"]; !ok {
		// no error message was extracted, find alternatives
		if status.Message() != "" {
			// use the status message
			metaMap["error.msg"] = status.Message()
		} else {
			// Use semantic registry lookup for http.status_code
			// `http.status_code` was renamed to `http.response.status_code` in the HTTP stabilization from v1.23.
			// See https://opentelemetry.io/docs/specs/semconv/http/migration-guide/#summary-of-changes
			accessor := semantics.NewStringMapAccessor(metaMap)
			httpCode := semantics.LookupString(semantics.DefaultRegistry(), accessor, semantics.ConceptHTTPStatusCode)
			if httpCode != "" {
				httpCodeInt, _ := strconv.Atoi(httpCode) // Value returned in case of error will not pass the next test
				if httptext := http.StatusText(httpCodeInt); httptext != "" {
					metaMap["error.msg"] = fmt.Sprintf("%s %s", httpCode, httptext)
				} else {
					metaMap["error.msg"] = httpCode
				}
			}
		}
	}
	return 1
}

// GetFirstFromMap checks each key in the given keys in the map and returns the first key-value pair whose
// key matches, or empty strings if none matches.
func GetFirstFromMap(m map[string]string, keys ...string) (string, string) {
	for _, key := range keys {
		if val := m[key]; val != "" {
			return key, val
		}
	}
	return "", ""
}

func spanMetaHasKey(s *pb.Span, k string) bool {
	_, ok := s.Meta[k]
	return ok
}
