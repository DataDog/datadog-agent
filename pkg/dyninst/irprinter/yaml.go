// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irprinter

import (
	"fmt"

	"github.com/go-json-experiment/json"
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
	var vv any
	if err := json.Unmarshal(marshaledJSON, &vv,
		json.WithUnmarshalers(json.UnmarshalFromFunc(anyToYaml)),
	); err != nil {
		return nil, err
	}
	marshaledYAML, err := yaml.Marshal(vv)
	if err != nil {
		return nil, err
	}
	return marshaledYAML, nil
}

func anyToYaml(dec *jsontext.Decoder, v *any) error {
	kind := dec.PeekKind()
	if kind != '{' {
		return json.SkipFunc
	}
	_, err := dec.ReadToken()
	if err != nil {
		return err
	}
	out := yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
	for {
		kind := dec.PeekKind()
		if kind == '}' {
			break
		}
		if kind != '"' {
			return fmt.Errorf("expected string, got %s", kind)
		}
		var key string
		{
			if err := json.UnmarshalDecode(dec, &key); err != nil {
				return err
			}
			n := yaml.Node{}
			if err := n.Encode(key); err != nil {
				return err
			}
			out.Content = append(out.Content, &n)
		}
		{
			var vv any
			before := dec.InputOffset()
			if err := json.UnmarshalDecode(dec, &vv); err != nil {
				return err
			}
			after := dec.InputOffset()
			if slice, ok := vv.([]any); ok {
				var yamlSlice []*yaml.Node
				for _, v := range slice {
					n := yaml.Node{}
					if err := n.Encode(v); err != nil {
						return fmt.Errorf("failed to encode %v %T: %w", v, v, err)
					}
					yamlSlice = append(yamlSlice, &n)
				}
				var listNode yaml.Node
				if err := listNode.Encode(yamlSlice); err != nil {
					return fmt.Errorf("failed to encode %v %T: %w", vv, vv, err)
				}
				vv = &listNode
			}
			n := yaml.Node{}
			if err := n.Encode(vv); err != nil {
				return fmt.Errorf("failed to encode %v %T: %w", vv, vv, err)
			}
			if after-before < 60 {
				n.Style = yaml.FlowStyle
			}
			out.Content = append(out.Content, &n)
		}
	}
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '}' {
		return fmt.Errorf("expected }, got %s", tok.Kind())
	}
	*v = out
	return nil
}
