// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"io"
	"strings"

	"github.com/rickar/props"

	"gopkg.in/yaml.v3"
)

// flattener is a type used to custom unmarshal a yaml document to a map flat keys
type flattener struct {
	props map[string]string
}

// visit the current node recursively to convert to flattened properties.
// The function will only recurse yaml.MappingNode and handle yaml.ScalarNode values.
// This because this method is tailored to extract simple scalar values only like `spring.application.name`
func (f *flattener) visit(path []string, node *yaml.Node) {
	if len(node.Content)%2 != 0 {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		current := node.Content[i]
		next := node.Content[i+1]
		if current.Kind != yaml.ScalarNode || !(next.Kind == yaml.ScalarNode || next.Kind == yaml.MappingNode) {
			continue
		}
		path = append(path, current.Value)

		if next.Kind == yaml.MappingNode {
			f.visit(path, next)
		} else {
			f.props[strings.Join(path, ".")] = next.Value // no need to decode... only support primitives not array
		}
		path = path[:len(path)-1]

	}
}

// UnmarshalYAML custom yaml unmarshal
func (f *flattener) UnmarshalYAML(value *yaml.Node) error {
	f.visit([]string{}, value)
	return nil
}

// newYamlSource creates a mapSource property getter by parsing the content accessed by the reader
func newYamlSource(reader io.Reader) (props.PropertyGetter, error) {
	f := flattener{props: map[string]string{}}
	err := yaml.NewDecoder(reader).Decode(&f)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return &mapSource{m: f.props}, nil
}
