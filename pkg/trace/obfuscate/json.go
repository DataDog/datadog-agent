// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// obfuscateJSON obfuscates the given span's tag using the given obfuscator. If the obfuscator is
// nil it is considered disabled.
func (o *Obfuscator) obfuscateJSON(span *pb.Span, tag string, obfuscator *jsonObfuscator) {
	if obfuscator == nil || span.Meta == nil || span.Meta[tag] == "" {
		// obfuscator is disabled or tag is not present
		return
	}
	span.Meta[tag], _ = obfuscator.obfuscate([]byte(span.Meta[tag]))
	// we should accept whatever the obfuscator returns, even if it's an error: a parsing
	// error simply means that the JSON was invalid, meaning that we've only obfuscated
	// as much of it as we could. It is safe to accept the output, even if partial.
}

type valueTransformer func(string) string

type jsonObfuscator struct {
	keepKeys      map[string]bool // these keys will not be obfuscated
	transformKeys map[string]bool // these keys pass through the transformer
	transformer   valueTransformer

	scan     *scanner // scanner
	closures []bool   // closure stack, true if object (e.g. {[{ => []bool{true, false, true})
	key      bool     // true if scanning a key

	wiped             bool // true if obfuscation string (`"?"`) was already written for current value
	keeping           bool // true if not obfuscating
	transformingValue bool // true if collecting the next literal for transformation
	keepDepth         int  // the depth at which we've stopped obfuscating
}

func (o *Obfuscator) newJSONObfuscator(cfg *config.JSONObfuscationConfig) *jsonObfuscator {
	keepValue := make(map[string]bool, len(cfg.KeepValues))
	for _, v := range cfg.KeepValues {
		keepValue[v] = true
	}
	var transformValues map[string]bool
	var transformer valueTransformer
	if cfg.TransformerType == "obfuscate_sql" {
		transformer = o.ObfuscateSQLStringSafe
	} else {
		log.Warnf("unknown JSON value transformer type %s", cfg.TransformerType)
	}
	if transformer != nil {
		transformValues = make(map[string]bool, len(cfg.TransformValues))
		for _, v := range cfg.TransformValues {
			transformValues[v] = true
		}
	}
	return &jsonObfuscator{
		closures:      []bool{},
		keepKeys:      keepValue,
		transformKeys: transformValues,
		transformer:   transformer,
		scan:          &scanner{},
	}
}

// setKey verifies if we are currently scanning a key based on the current state
// and updates the state accordingly. It must be called only after a closure or a
// value scan has ended.
func (p *jsonObfuscator) setKey() {
	n := len(p.closures)
	p.key = n == 0 || p.closures[n-1] // true if we are at top level or in an object
	p.wiped = false
}

func trimSideQuotes(quotedString []byte) []byte {
	if len(quotedString) > 0 && quotedString[0] == '"' {
		quotedString = quotedString[1:]
	}
	if len(quotedString) > 0 && quotedString[len(quotedString)-1] == '"' {
		quotedString = quotedString[0 : len(quotedString)-1]
	}
	return quotedString
}

func (p *jsonObfuscator) obfuscate(data []byte) (string, error) {
	var out strings.Builder

	keyBuf := make([]byte, 0, 10) // recording key token
	valBuf := make([]byte, 0, 10) // recording value
	p.scan.reset()
	for _, c := range data {
		p.scan.bytes++
		op := p.scan.step(p.scan, c)
		depth := len(p.closures)
		switch op {
		case scanBeginObject:
			// object begins: {
			p.closures = append(p.closures, true)
			p.setKey()
			p.transformingValue = false

		case scanBeginArray:
			// array begins: [
			p.closures = append(p.closures, false)
			p.setKey()
			p.transformingValue = false

		case scanEndArray, scanEndObject:
			// array or object closing
			if n := len(p.closures) - 1; n > 0 {
				p.closures = p.closures[:n]
			}
			fallthrough

		case scanObjectValue, scanArrayValue:
			// done scanning value
			p.setKey()
			if p.transformingValue && p.transformer != nil {
				result := p.transformer(string(trimSideQuotes(valBuf)))
				out.Write([]byte(`"`))
				out.Write([]byte(result))
				out.Write([]byte(`"`))
				p.transformingValue = false
				valBuf = valBuf[:0]
			} else if p.keeping && depth < p.keepDepth {
				p.keeping = false
			}

		case scanBeginLiteral, scanContinue:
			// starting or continuing a literal
			if p.transformingValue {
				valBuf = append(valBuf, c)
				continue
			} else if p.key {
				// it's a key
				keyBuf = append(keyBuf, c)
			} else if !p.keeping {
				// it's a value we're not keeping
				if !p.wiped {
					out.Write([]byte(`"?"`))
					p.wiped = true
				}
				continue
			}

		case scanObjectKey:
			// done scanning key
			k := strings.Trim(string(keyBuf), `"`)
			if !p.keeping && p.keepKeys[k] {
				// we should not obfuscate values of this key
				p.keeping = true
				p.keepDepth = depth + 1
			} else if !p.transformingValue && p.transformer != nil && p.transformKeys[k] {
				// the string value immediately following this key will be passed through the value transformer
				// if anything other than a literal is found then sql obfuscation is stopped and json obfuscation
				// proceeds as usual
				p.transformingValue = true
			}

			keyBuf = keyBuf[:0]
			p.key = false

		case scanSkipSpace:
			continue

		case scanError:
			// we've encountered an error, mark that there might be more JSON
			// using the ellipsis and return whatever we've managed to obfuscate
			// thus far.
			out.Write([]byte("..."))
			return out.String(), p.scan.err
		}
		out.WriteByte(c)
	}
	if p.scan.eof() == scanError {
		// if an error occurred it's fine, simply add the ellipsis to indicate
		// that the input has been truncated.
		out.Write([]byte("..."))
		return out.String(), p.scan.err
	}
	return out.String(), nil
}

func stringOp(op int) string {
	return [...]string{
		"Continue",
		"BeginLiteral",
		"BeginObject",
		"ObjectKey",
		"ObjectValue",
		"EndObject",
		"BeginArray",
		"ArrayValue",
		"EndArray",
		"SkipSpace",
		"End",
		"Error",
	}[op]
}
