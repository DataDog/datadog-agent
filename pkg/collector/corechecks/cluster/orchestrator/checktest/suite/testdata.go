// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yamlv3 "go.yaml.in/yaml/v3"
	"sigs.k8s.io/yaml"
)

// testDataDir holds a file for each resource, named after the resource, like deployments.yaml, relative to the package
// of the tests. The resource name alone keeps the paths short enough for the Windows build.
//
// A file has a section for each object of the resource, by UID, like apps_v1_deployments: its manifest, the object of
// the default cluster, and its live and deleted payloads, what the check sends for it. A UID is the full name of the
// collector of the object, with underscores for slashes, so the versions of a resource share its file.
const testDataDir = "testdata"

// sectionKeys are the keys of the section of an object, in the order of the file.
var sectionKeys = []string{"manifest", "live", "deleted"}

// testDataPath returns the file of the object with the UID: the last part of the UID is its resource.
func testDataPath(uid string) string {
	return filepath.Join(testDataDir, uid[strings.LastIndex(uid, "_")+1:]+".yaml")
}

// readSections returns the sections of a test data file, by UID, with the raw JSON of each key. A missing file has no
// section.
func readSections(path string) (map[string]map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sections map[string]map[string]json.RawMessage
	if err := yaml.Unmarshal(data, &sections); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return sections, nil
}

// writeSection sets one key of the section of an object in its test data file. It keeps the rest of the file as it
// is, so that -update leaves the hand-written manifests unchanged.
func writeSection(uid, key string, value any) error {
	path := testDataPath(uid)
	root := &yamlv3.Node{Kind: yamlv3.MappingNode}
	if data, err := os.ReadFile(path); err == nil {
		var doc yamlv3.Node
		if err := yamlv3.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if len(doc.Content) > 0 {
			root = doc.Content[0]
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// The JSON round trip of sigs.k8s.io/yaml writes numbers and strings like the rest of the payloads.
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	var encoded yamlv3.Node
	if err := yamlv3.Unmarshal(data, &encoded); err != nil {
		return err
	}
	section := mappingValue(root, uid, strings.Compare)
	if section.Kind == 0 {
		section.Kind = yamlv3.MappingNode
	}
	*mappingValue(section, key, compareSectionKeys) = *encoded.Content[0]

	var out bytes.Buffer
	encoder := yamlv3.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// mappingValue returns the value of the key in a mapping node, and inserts an empty one in the order of compare when
// the key is missing.
func mappingValue(mapping *yamlv3.Node, key string, compare func(a, b string) int) *yamlv3.Node {
	at := len(mapping.Content)
	for i := 0; i < len(mapping.Content); i += 2 {
		existing := mapping.Content[i].Value
		if existing == key {
			return mapping.Content[i+1]
		}
		if at == len(mapping.Content) && compare(key, existing) < 0 {
			at = i
		}
	}
	keyNode := &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: key}
	value := &yamlv3.Node{}
	mapping.Content = slices.Insert(mapping.Content, at, keyNode, value)
	return value
}

func compareSectionKeys(a, b string) int {
	return slices.Index(sectionKeys, a) - slices.Index(sectionKeys, b)
}
