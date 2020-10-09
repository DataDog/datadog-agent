// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/config/configdefs"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
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

type jsonObfuscator struct {
	keepers map[string]bool // these keys will not be obfuscated

	scan     *scanner // scanner
	closures []bool   // closure stack, true if object (e.g. {[{ => []bool{true, false, true})
	key      bool     // true if scanning a key

	wiped     bool // true if obfuscation string (`"?"`) was already written for current value
	keeping   bool // true if not obfuscating
	keepDepth int  // the depth at which we've stopped obfuscating
}

func newJSONObfuscator(cfg *configdefs.JSONObfuscationConfig) *jsonObfuscator {
	keepValue := make(map[string]bool, len(cfg.KeepValues))
	for _, v := range cfg.KeepValues {
		keepValue[v] = true
	}
	return &jsonObfuscator{
		closures: []bool{},
		keepers:  keepValue,
		scan:     &scanner{},
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

func (p *jsonObfuscator) obfuscate(data []byte) (string, error) {
	var out strings.Builder
	buf := make([]byte, 0, 10) // recording key token
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

		case scanBeginArray:
			// array begins: [
			p.closures = append(p.closures, false)
			p.setKey()

		case scanEndArray, scanEndObject:
			// array or object closing
			if n := len(p.closures) - 1; n > 0 {
				p.closures = p.closures[:n]
			}
			fallthrough

		case scanObjectValue, scanArrayValue:
			// done scanning value
			p.setKey()
			if p.keeping && depth < p.keepDepth {
				p.keeping = false
			}

		case scanBeginLiteral, scanContinue:
			// starting or continuing a literal
			if p.key {
				// it's a key
				buf = append(buf, c)
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
			k := strings.Trim(string(buf), `"`)
			if !p.keeping && p.keepers[k] {
				// we should not obfuscate values of this key
				p.keeping = true
				p.keepDepth = depth + 1
			}
			buf = buf[:0]
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
