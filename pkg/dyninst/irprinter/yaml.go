// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irprinter

import (
	"bytes"
	"fmt"

	"github.com/go-json-experiment/json/jsontext"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// PrintYAML prints the IR program to YAML.
func PrintYAML(p *ir.Program) ([]byte, error) {
	marshaledJSON, err := PrintJSON(p)
	if err != nil {
		return nil, err
	}
	dec := jsontext.NewDecoder(bytes.NewReader(marshaledJSON))
	root, err := jsonStreamToYAMLNode(dec)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(root)
}

// flowStyleByteLimit is the JSON byte length below which a value is rendered
// with YAML flow style. Matches the prior behaviour of anyToYaml: short
// composite values inline, long ones use block style.
const flowStyleByteLimit = 60

// isYAML11Bool reports whether s is a string that YAML 1.1 parsers would
// interpret as a boolean. The yaml-v3 emitter quotes such strings when
// encoding from a Go string so the output stays YAML-1.1-safe; we replicate
// that for parity since the Node-path encode does not.
func isYAML11Bool(s string) bool {
	switch s {
	case "y", "Y", "yes", "Yes", "YES", "on", "On", "ON",
		"n", "N", "no", "No", "NO", "off", "Off", "OFF":
		return true
	}
	return false
}

// jsonStreamToYAMLNode reads a single JSON value from dec and returns the
// corresponding yaml.Node tree. It does not allocate an intermediate `any`
// representation: scalars become ScalarNode, arrays become SequenceNode,
// objects become MappingNode whose Content preserves JSON key order.
func jsonStreamToYAMLNode(dec *jsontext.Decoder) (*yaml.Node, error) {
	kind := dec.PeekKind()
	switch kind {
	case '{':
		return readObjectNode(dec)
	case '[':
		return readArrayNode(dec)
	case '"', '0', 't', 'f', 'n':
		return readScalarNode(dec)
	default:
		return nil, fmt.Errorf("unexpected JSON kind: %s", kind)
	}
}

func readObjectNode(dec *jsontext.Decoder) (*yaml.Node, error) {
	if _, err := dec.ReadToken(); err != nil { // consume '{'
		return nil, err
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for dec.PeekKind() != '}' {
		keyTok, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if keyTok.Kind() != '"' {
			return nil, fmt.Errorf("expected JSON object key, got %s", keyTok.Kind())
		}
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: keyTok.String(),
		}
		// Map values get flow style applied if their JSON form is short
		// enough. This matches the prior behaviour where short objects
		// and arrays were inlined as map values.
		valBegin := dec.InputOffset()
		valNode, err := jsonStreamToYAMLNode(dec)
		if err != nil {
			return nil, err
		}
		if (valNode.Kind == yaml.MappingNode || valNode.Kind == yaml.SequenceNode) &&
			dec.InputOffset()-valBegin < flowStyleByteLimit {
			valNode.Style = yaml.FlowStyle
		}
		out.Content = append(out.Content, keyNode, valNode)
	}
	if _, err := dec.ReadToken(); err != nil { // consume '}'
		return nil, err
	}
	return out, nil
}

func readArrayNode(dec *jsontext.Decoder) (*yaml.Node, error) {
	if _, err := dec.ReadToken(); err != nil { // consume '['
		return nil, err
	}
	out := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for dec.PeekKind() != ']' {
		elemNode, err := jsonStreamToYAMLNode(dec)
		if err != nil {
			return nil, err
		}
		out.Content = append(out.Content, elemNode)
	}
	if _, err := dec.ReadToken(); err != nil { // consume ']'
		return nil, err
	}
	return out, nil
}

func readScalarNode(dec *jsontext.Decoder) (*yaml.Node, error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	n := &yaml.Node{Kind: yaml.ScalarNode}
	switch tok.Kind() {
	case '"':
		// Set !!str so the encoder runs its resolve-based quoting
		// logic: if the string looks like another YAML type (e.g.
		// "0x4af3c6" reads as an int, "" as null), it emits it
		// double-quoted; otherwise plain. The encoder's Node-path
		// resolve does not detect YAML 1.1 booleans like "y" or "no",
		// so handle those by forcing DoubleQuotedStyle directly.
		n.Tag = "!!str"
		n.Value = tok.String()
		if isYAML11Bool(n.Value) {
			n.Style = yaml.DoubleQuotedStyle
		}
	case '0':
		// Numbers are unambiguous as plain scalars; keep the original
		// JSON spelling (preserves integer form rather than reformatting
		// through float64).
		n.Value = tok.String()
	case 't', 'f':
		n.Tag = "!!bool"
		if tok.Bool() {
			n.Value = "true"
		} else {
			n.Value = "false"
		}
	case 'n':
		n.Tag = "!!null"
		n.Value = "null"
	default:
		return nil, fmt.Errorf("unexpected scalar JSON kind: %s", tok.Kind())
	}
	return n, nil
}
