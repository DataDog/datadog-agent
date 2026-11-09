// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

const (
	tagRedisRawCommand  = transform.TagRedisRawCommand
	tagValkeyRawCommand = transform.TagValkeyRawCommand
	tagMemcachedCommand = transform.TagMemcachedCommand
	tagMongoDBQuery     = transform.TagMongoDBQuery
	tagElasticBody      = transform.TagElasticBody
	tagOpenSearchBody   = transform.TagOpenSearchBody
	tagSQLQuery         = transform.TagSQLQuery
	tagHTTPURL          = transform.TagHTTPURL
	tagDBMS             = transform.TagDBMS

	tagSQLTables = "sql.tables"
)

// OTel semantic convention attributes carrying the raw database query, which
// must be obfuscated like the resource of SQL spans.
const (
	tagDBStatement = "db.statement"  // semconv v1.6.1 DBStatementKey
	tagDBQueryText = "db.query.text" // semconv v1.26.0 DBQueryTextKey
)

const (
	textNonParsable = transform.TextNonParsable
)

// obfuscateSpan is an interface that exposes all the methods needed to obfuscate a span.
type obfuscateSpan interface {
	GetAttributeAsString(key string) (string, bool)
	SetStringAttribute(key string, value string)
	DeleteAttribute(key string)
	Type() string
	Resource() string
	SetResource(resource string)
	Service() string
	// MapFilteredAttributes maps over all attributes where shouldMap returns true and applies the given function to each attribute
	MapFilteredAttributes(shouldMap func(k string) bool, mapper func(k, v string) string)
}

type obfuscateSpanV0 struct {
	span *pb.Span
}

func (o *obfuscateSpanV0) GetAttributeAsString(key string) (string, bool) {
	v, ok := o.span.Meta[key]
	return v, ok
}

func (o *obfuscateSpanV0) SetStringAttribute(key string, value string) {
	if o.span.Meta == nil {
		o.span.Meta = make(map[string]string)
	}
	o.span.Meta[key] = value
}

func (o *obfuscateSpanV0) DeleteAttribute(key string) {
	delete(o.span.Meta, key)
}

func (o *obfuscateSpanV0) Type() string {
	return o.span.Type
}

func (o *obfuscateSpanV0) Resource() string {
	return o.span.Resource
}

func (o *obfuscateSpanV0) SetResource(resource string) {
	o.span.Resource = resource
}

func (o *obfuscateSpanV0) Service() string {
	return o.span.Service
}

func (o *obfuscateSpanV0) MapFilteredAttributes(shouldMap func(k string) bool, mapper func(k, v string) string) {
	for k, v := range o.span.Meta {
		if !shouldMap(k) {
			continue
		}
		newV := mapper(k, v)
		if newV != v {
			o.span.Meta[k] = newV
		}
	}
}

// ObfuscateSQLSpan obfuscates a SQL span
func ObfuscateSQLSpan(o *obfuscate.Obfuscator, span *pb.Span) (*obfuscate.ObfuscatedQuery, error) {
	return obfuscateSQLSpan(o, &obfuscateSpanV0{span: span})
}

// sqlQueryAttributes are the span attributes that may carry the raw SQL query
// and are obfuscated along with the resource of SQL spans, when present.
var sqlQueryAttributes = [...]string{tagSQLQuery, tagDBStatement, tagDBQueryText}

// obfuscateSQLSpan obfuscates the resource of a SQL span along with the
// attributes that carry the raw SQL query (if sent by the client).
func obfuscateSQLSpan(o *obfuscate.Obfuscator, span obfuscateSpan) (*obfuscate.ObfuscatedQuery, error) {
	dbms, _ := span.GetAttributeAsString(tagDBMS)
	rawResource := span.Resource()
	if rawResource == "" {
		obfuscateSQLAttributes(o, span, dbms, "", "")
		return nil, nil
	}
	oq, err := o.ObfuscateSQLStringForDBMS(rawResource, dbms)
	if err != nil {
		// we have an error, discard the SQL to avoid polluting user resources.
		span.SetResource(textNonParsable)
		obfuscateSQLAttributes(o, span, dbms, rawResource, textNonParsable)
		return nil, err
	}
	span.SetResource(oq.Query)
	obfuscateSQLAttributes(o, span, dbms, rawResource, oq.Query)
	if len(oq.Metadata.TablesCSV) > 0 {
		span.SetStringAttribute(tagSQLTables, oq.Metadata.TablesCSV)
	}
	return oq, nil
}

// obfuscateSQLAttributes runs obfuscateSQLAttribute for each of sqlQueryAttributes.
func obfuscateSQLAttributes(o *obfuscate.Obfuscator, span obfuscateSpan, dbms, rawResource, obfuscatedResource string) {
	for _, key := range sqlQueryAttributes {
		obfuscateSQLAttribute(o, span, key, dbms, rawResource, obfuscatedResource)
	}
}

// obfuscateSQLAttribute obfuscates the SQL query stored under key, if present
// and non-empty. A value equal to the raw resource reuses obfuscatedResource;
// any other value is obfuscated separately and replaced by textNonParsable if
// that fails, so the raw value is never kept.
func obfuscateSQLAttribute(o *obfuscate.Obfuscator, span obfuscateSpan, key, dbms, rawResource, obfuscatedResource string) {
	v, ok := span.GetAttributeAsString(key)
	if ok && v == "" && key == tagSQLQuery {
		// An empty sql.query carries no query. Drop it so the intake derives
		// db.statement from the resource, as it did when the agent overwrote it.
		span.DeleteAttribute(key)
		return
	}
	if !ok || v == "" {
		return
	}
	if v == rawResource {
		span.SetStringAttribute(key, obfuscatedResource)
		return
	}
	obfuscated := textNonParsable
	if oq, err := o.ObfuscateSQLStringForDBMS(v, dbms); err == nil {
		obfuscated = oq.Query
	}
	span.SetStringAttribute(key, obfuscated)
}

// ObfuscateRedisSpan obfuscates a Redis span
func ObfuscateRedisSpan(o *obfuscate.Obfuscator, span *pb.Span, removeAllArgs bool) {
	obfuscateRedisSpan(o, &obfuscateSpanV0{span: span}, removeAllArgs)
}

func obfuscateRedisSpan(o *obfuscate.Obfuscator, span obfuscateSpan, removeAllArgs bool) {
	v, ok := span.GetAttributeAsString(tagRedisRawCommand)
	if !ok || v == "" {
		return
	}
	if removeAllArgs {
		span.SetStringAttribute(tagRedisRawCommand, o.RemoveAllRedisArgs(v))
		return
	}
	span.SetStringAttribute(tagRedisRawCommand, o.ObfuscateRedisString(v))
}

// ObfuscateValkeySpan obfuscates a Valkey span
func ObfuscateValkeySpan(o *obfuscate.Obfuscator, span *pb.Span, removeAllArgs bool) {
	obfuscateValkeySpan(o, &obfuscateSpanV0{span: span}, removeAllArgs)
}

func obfuscateValkeySpan(o *obfuscate.Obfuscator, span obfuscateSpan, removeAllArgs bool) {
	v, ok := span.GetAttributeAsString(tagValkeyRawCommand)
	if !ok || v == "" {
		return
	}
	if removeAllArgs {
		span.SetStringAttribute(tagValkeyRawCommand, o.RemoveAllRedisArgs(v))
		return
	}
	span.SetStringAttribute(tagValkeyRawCommand, o.ObfuscateRedisString(v))
}

func (a *Agent) obfuscateSpanInternal(span obfuscateSpan) {
	o := a.lazyInitObfuscator()
	if a.conf.Obfuscation != nil && a.conf.Obfuscation.CreditCards.Enabled {
		span.MapFilteredAttributes(o.ShouldObfuscateCCKey, func(k, v string) string {
			newV := o.ObfuscateCreditCardNumber(v)
			if newV != v {
				log.Debugf("obfuscating possible credit card under key %s from service %s", k, span.Service())
				return newV
			}
			return v
		})
	}

	switch span.Type() {
	case "sql", "cassandra":
		if _, err := obfuscateSQLSpan(o, span); err != nil {
			log.Debugf("Error parsing SQL query: %v", err)
		}
	case "redis", "valkey":
		// if a span is redis/valkey type, it should be quantized regardless of obfuscation setting.
		// valkey is a folk of redis, so we can use the same logic for both.
		span.SetResource(o.QuantizeRedisString(span.Resource()))
		if span.Type() == "redis" && a.conf.Obfuscation.Redis.Enabled {
			obfuscateRedisSpan(o, span, a.conf.Obfuscation.Redis.RemoveAllArgs)
		}
		if span.Type() == "valkey" && a.conf.Obfuscation.Valkey.Enabled {
			obfuscateValkeySpan(o, span, a.conf.Obfuscation.Valkey.RemoveAllArgs)
		}
	case "memcached":
		if !a.conf.Obfuscation.Memcached.Enabled {
			return
		}
		v, ok := span.GetAttributeAsString(tagMemcachedCommand)
		if !ok || v == "" {
			return
		}
		span.SetStringAttribute(tagMemcachedCommand, o.ObfuscateMemcachedString(v))
	case "web", "http":
		v, ok := span.GetAttributeAsString(tagHTTPURL)
		if !ok || v == "" {
			return
		}
		span.SetStringAttribute(tagHTTPURL, o.ObfuscateURLString(v))
	case "mongodb":
		if !a.conf.Obfuscation.Mongo.Enabled {
			return
		}
		v, ok := span.GetAttributeAsString(tagMongoDBQuery)
		if !ok || v == "" {
			return
		}
		span.SetStringAttribute(tagMongoDBQuery, o.ObfuscateMongoDBString(v))
	case "elasticsearch", "opensearch":
		if a.conf.Obfuscation.ES.Enabled {
			v, ok := span.GetAttributeAsString(tagElasticBody)
			if ok && v != "" {
				span.SetStringAttribute(tagElasticBody, o.ObfuscateElasticSearchString(v))
			}
		}
		if a.conf.Obfuscation.OpenSearch.Enabled {
			v, ok := span.GetAttributeAsString(tagOpenSearchBody)
			if !ok || v == "" {
				return
			}
			span.SetStringAttribute(tagOpenSearchBody, o.ObfuscateOpenSearchString(v))
		}
	}
}

func (a *Agent) ObfuscateSpan(span *pb.Span) {
	a.lazyInitObfuscator()
	for _, spanEvent := range span.SpanEvents {
		a.obfuscateSpanEvent(spanEvent)
	}
	a.obfuscateSpanInternal(&obfuscateSpanV0{span: span})
}

// obfuscateSpanEvent uses the pre-configured agent obfuscator to do limited obfuscation of span events
// For now, we only obfuscate any credit-card like when enabled.
func (a *Agent) obfuscateSpanEvent(spanEvent *pb.SpanEvent) {
	if a.conf.Obfuscation != nil && a.conf.Obfuscation.CreditCards.Enabled && spanEvent != nil {
		for k, v := range spanEvent.Attributes {
			if !a.obfuscator.ShouldObfuscateCCKey(k) {
				continue
			}
			var strValue string
			switch v.Type {
			case pb.AttributeAnyValue_STRING_VALUE:
				strValue = v.StringValue
			case pb.AttributeAnyValue_DOUBLE_VALUE:
				strValue = strconv.FormatFloat(v.DoubleValue, 'f', -1, 64)
			case pb.AttributeAnyValue_INT_VALUE:
				strValue = strconv.FormatInt(v.IntValue, 10)
			case pb.AttributeAnyValue_BOOL_VALUE:
				continue // Booleans can't be credit cards
			case pb.AttributeAnyValue_ARRAY_VALUE:
				a.ccObfuscateAttributeArray(v)
			}
			newVal := a.obfuscator.ObfuscateCreditCardNumber(strValue)
			if newVal != strValue {
				*v = pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: newVal}
			}
		}
	}
}

func (a *Agent) ccObfuscateAttributeArray(v *pb.AttributeAnyValue) {
	// The attribute may declare ARRAY_VALUE while carrying a nil ArrayValue,
	// since Type and ArrayValue are independent fields rather than a real oneof.
	if v.ArrayValue == nil {
		return
	}
	var arrStrValue string
	for _, vElement := range v.ArrayValue.Values {
		if vElement == nil {
			continue
		}
		switch vElement.Type {
		case pb.AttributeArrayValue_STRING_VALUE:
			arrStrValue = vElement.StringValue
		case pb.AttributeArrayValue_DOUBLE_VALUE:
			arrStrValue = strconv.FormatFloat(vElement.DoubleValue, 'f', -1, 64)
		case pb.AttributeArrayValue_INT_VALUE:
			arrStrValue = strconv.FormatInt(vElement.IntValue, 10)
		case pb.AttributeArrayValue_BOOL_VALUE:
			continue // Booleans can't be credit cards
		}
		newVal := a.obfuscator.ObfuscateCreditCardNumber(arrStrValue)
		if newVal != arrStrValue {
			*vElement = pb.AttributeArrayValue{Type: pb.AttributeArrayValue_STRING_VALUE, StringValue: newVal}
		}
	}
}

func (a *Agent) obfuscateStatsGroup(b *pb.ClientGroupedStats) {
	o := a.lazyInitObfuscator()

	switch b.Type {
	case "sql", "cassandra":
		oq, err := o.ObfuscateSQLStringForDBMS(b.Resource, b.DBType)
		if err != nil {
			log.Errorf("Error obfuscating stats group resource %q: %v", b.Resource, err)
			b.Resource = textNonParsable
		} else {
			b.Resource = oq.Query
		}
	case "redis", "valkey":
		b.Resource = o.QuantizeRedisString(b.Resource)
	}
}

var obfuscatorLock sync.Mutex

func (a *Agent) lazyInitObfuscator() *obfuscate.Obfuscator {
	// Ensure thread safe initialization
	obfuscatorLock.Lock()
	defer obfuscatorLock.Unlock()

	if a.obfuscator == nil {
		if a.obfuscatorConf != nil {
			a.obfuscator = obfuscate.NewObfuscator(*a.obfuscatorConf)
		} else {
			a.obfuscator = obfuscate.NewObfuscator(obfuscate.Config{})
		}
	}

	return a.obfuscator
}
