// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
)

// ObfuscateMongoDBString obfuscates the given MongoDB JSON query.
func (o *Obfuscator) ObfuscateMongoDBString(cmd string) string {
	return obfuscateJSONString(cmd, o.mongo)
}

// ObfuscateElasticSearchString obfuscates the given ElasticSearch JSON query.
func (o *Obfuscator) ObfuscateElasticSearchString(cmd string) string {
	return obfuscateJSONString(cmd, o.es)
}

// obfuscateJSONString obfuscates the given span's tag using the given obfuscator. If the obfuscator is
// nil it is considered disabled.
func obfuscateJSONString(cmd string, obfuscator *jsonObfuscator) string {
	if obfuscator == nil || cmd == "" {
		// obfuscator is disabled or string is empty
		return cmd
	}
	out, _ := obfuscator.obfuscate([]byte(cmd))
	// we should accept whatever the obfuscator returns, even if it's an error: a parsing
	// error simply means that the JSON was invalid, meaning that we've only obfuscated
	// as much of it as we could. It is safe to accept the output, even if partial.
	return out
}

type jsonObfuscator struct {
	buffPool      sync.Pool       // pool for fixed-length buffers (50 showed to be the optimal running benchmarks with different length)
	statePool     sync.Pool       // pool for jsonObfuscatorState values
	keepKeys      map[string]bool // the values for these keys will not be obfuscated
	transformKeys map[string]bool // the values for these keys pass through the transformer
	transformer   func(string) string
}

func newJSONObfuscator(cfg *JSONConfig, o *Obfuscator) *jsonObfuscator {
	keepValue := make(map[string]bool, len(cfg.KeepValues))
	for _, v := range cfg.KeepValues {
		keepValue[v] = true
	}
	var (
		transformKeys map[string]bool
		transformer   func(string) string
	)
	if len(cfg.ObfuscateSQLValues) > 0 {
		transformer = sqlObfuscationTransformer(o)
		transformKeys = make(map[string]bool, len(cfg.ObfuscateSQLValues))
		for _, v := range cfg.ObfuscateSQLValues {
			transformKeys[v] = true
		}
	}
	return &jsonObfuscator{
		keepKeys:      keepValue,
		transformKeys: transformKeys,
		transformer:   transformer,
		buffPool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
		statePool: sync.Pool{
			New: func() any {
				return &jsonObfuscatorState{
					closures: []bool{},
				}
			},
		},
	}
}

func sqlObfuscationTransformer(o *Obfuscator) func(string) string {
	return func(s string) string {
		result, err := o.ObfuscateSQLString(s)
		if err != nil {
			o.log.Debugf("Failed to obfuscate SQL string '%s': %s", s, err.Error())
			// instead of returning an empty string we explicitly return an error string here within the result in order
			// to surface the problem clearly to the user
			return "Datadog-agent failed to obfuscate SQL string. Enable agent debug logs for more info."
		}
		return result.Query
	}
}

type jsonObfuscatorState struct {
	scan              scanner // scanner
	closures          []bool  // closure stack, true if object (e.g. {[{ => []bool{true, false, true})
	keepDepth         int     // the depth at which we've stopped obfuscating
	key               bool    // true if scanning a key
	wiped             bool    // true if obfuscation string (`"?"`) was already written for current value
	keeping           bool    // true if not obfuscating
	transformingValue bool    // true if collecting the next literal for transformation
}

func (st *jsonObfuscatorState) reset() {
	st.scan.reset()
	st.closures = st.closures[0:0]
	st.keepDepth = 0
	st.key = false
	st.wiped = false
	st.keeping = false
	st.transformingValue = false
}

// setKey verifies if we are currently scanning a key based on the current state
// and updates the state accordingly. It must be called only after a closure or a
// value scan has ended.
func (st *jsonObfuscatorState) setKey() {
	n := len(st.closures)
	st.key = n == 0 || st.closures[n-1] // true if we are at top level or in an object
	st.wiped = false
}

func (p *jsonObfuscator) obfuscate(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	var out strings.Builder
	st := p.statePool.Get().(*jsonObfuscatorState)
	st.reset()

	buf := p.buffPool.Get().(*bytes.Buffer) // recording current token
	buf.Reset()
	defer func() {
		p.statePool.Put(st)
		p.buffPool.Put(buf)
	}()

	out.Grow(len(data))
	buf.Grow(len(data) / 10) // Benchmarks show that the optimal point is a tenth of the data length.
	for _, c := range data {
		st.scan.bytes++
		op := st.scan.step(&st.scan, c)
		depth := len(st.closures)
		switch op {
		case scanBeginObject:
			// object begins: {
			st.closures = append(st.closures, true)
			st.setKey()
			st.transformingValue = false
		case scanBeginArray:
			// array begins: [
			st.closures = append(st.closures, false)
			st.setKey()
			st.transformingValue = false
		case scanEndArray, scanEndObject:
			// array or object closing
			if n := len(st.closures) - 1; n > 0 {
				st.closures = st.closures[:n]
			}
			fallthrough
		case scanObjectValue, scanArrayValue:
			// done scanning value
			st.setKey()
			if st.transformingValue && p.transformer != nil {
				v, err := strconv.Unquote(buf.String())
				if err != nil {
					v = buf.String()
				}
				result := p.transformer(v)
				out.WriteByte('"')
				out.WriteString(result)
				out.WriteByte('"')
				st.transformingValue = false
				buf.Reset()
			} else if st.keeping && depth < st.keepDepth {
				st.keeping = false
			}
		case scanBeginLiteral, scanContinue:
			// starting or continuing a literal
			if st.transformingValue {
				buf.WriteByte(c)
				continue
			} else if st.key {
				// it's a key
				buf.WriteByte(c)
			} else if !st.keeping {
				// it's a value we're not keeping
				if !st.wiped {
					out.WriteString(`"?"`)
					st.wiped = true
				}
				continue
			}
		case scanObjectKey:
			// done scanning key
			k := string(bytes.Trim(buf.Bytes(), `"`))
			if !st.keeping && p.keepKeys[k] {
				// we should not obfuscate values of this key
				st.keeping = true
				st.keepDepth = depth + 1
			} else if !st.transformingValue && p.transformer != nil && p.transformKeys[k] {
				// the string value immediately following this key will be passed through the value transformer
				// if anything other than a literal is found then sql obfuscation is stopped and json obfuscation
				// proceeds as usual
				st.transformingValue = true
			}
			buf.Reset()
			st.key = false
		case scanSkipSpace:
			continue
		case scanError:
			// we've encountered an error, mark that there might be more JSON
			// using the ellipsis and return whatever we've managed to obfuscate
			// thus far.
			out.WriteString("...")
			return out.String(), st.scan.err
		}
		out.WriteByte(c)
	}
	if st.scan.eof() == scanError {
		// if an error occurred it's fine, simply add the ellipsis to indicate
		// that the input has been truncated.
		out.Write([]byte("..."))
		return out.String(), st.scan.err
	}
	return out.String(), nil
}
