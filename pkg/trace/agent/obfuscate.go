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
)

const (
	textNonParsable = transform.TextNonParsable
)

// obfuscateSpan is an interface that exposes all the methods needed to obfuscate a span.
type obfuscateSpan interface {
	GetAttributeAsString(key string) (string, bool)
	SetStringAttribute(key string, value string)
	Type() string
	Resource() string
	SetResource(resource string)
	Service() string
	// MapAttributesAsStrings applies a function to all string attributes of the span, if the function returns a non-empty string, the attribute is set to the new value
	MapAttributesAsStrings(func(k, v string) string)
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

func (o *obfuscateSpanV0) MapAttributesAsStrings(f func(k, v string) string) {
	for k, v := range o.span.Meta {
		newV := f(k, v)
		if newV != "" && newV != v {
			o.span.Meta[k] = newV
		}
	}
}

// ObfuscateSQLSpan obfuscates a SQL span
func ObfuscateSQLSpan(o *obfuscate.Obfuscator, span *pb.Span) (*obfuscate.ObfuscatedQuery, error) {
	return obfuscateSQLSpan(o, &obfuscateSpanV0{span: span})
}

func obfuscateSQLSpan(o *obfuscate.Obfuscator, span obfuscateSpan) (*obfuscate.ObfuscatedQuery, error) {
	if span.Resource() == "" {
		return nil, nil
	}
	dbms, _ := span.GetAttributeAsString(tagDBMS)
	oq, err := o.ObfuscateSQLStringForDBMS(span.Resource(), dbms)
	if err != nil {
		// we have an error, discard the SQL to avoid polluting user resources.
		span.SetResource(textNonParsable)
		span.SetStringAttribute(tagSQLQuery, textNonParsable)
		return nil, err
	}
	span.SetResource(oq.Query)
	if len(oq.Metadata.TablesCSV) > 0 {
		span.SetStringAttribute("sql.tables", oq.Metadata.TablesCSV)
	}
	span.SetStringAttribute(tagSQLQuery, oq.Query)
	return oq, nil
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
		span.MapAttributesAsStrings(func(k, v string) string {
			newV := o.ObfuscateCreditCardNumber(k, v)
			if newV != v {
				log.Debugf("obfuscating possible credit card under key %s from service %s", k, span.Service())
				return newV
			}
			return v
		})
	}

	switch span.Type() {
	case "sql", "cassandra":
		if span.Resource() == "" {
			return
		}
		oq, err := obfuscateSQLSpan(o, span)
		if err != nil {
			// we have an error, discard the SQL to avoid polluting user resources.
			log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
			return
		}
		if oq == nil {
			// no error was thrown but no query was found/sanitized either
			return
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
			if !ok || v == "" {
				return
			}
			span.SetStringAttribute(tagElasticBody, o.ObfuscateElasticSearchString(v))
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

func (a *Agent) obfuscateSpan(span *pb.Span) {
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
				a.ccObfuscateAttributeArray(v, k, strValue)
			}
			newVal := a.obfuscator.ObfuscateCreditCardNumber(k, strValue)
			if newVal != strValue {
				*v = pb.AttributeAnyValue{Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: newVal}
			}
		}
	}
}

func (a *Agent) ccObfuscateAttributeArray(v *pb.AttributeAnyValue, k string, strValue string) {
	var arrStrValue string
	for _, vElement := range v.ArrayValue.Values {
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
		newVal := a.obfuscator.ObfuscateCreditCardNumber(k, arrStrValue)
		if newVal != strValue {
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

var (
	obfuscatorLock sync.Mutex
)

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
