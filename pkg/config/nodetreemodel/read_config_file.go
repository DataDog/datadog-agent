// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"gopkg.in/yaml.v2"
)

func (c *ntmConfig) mergeAllLayers() error {
	root := newInnerNodeImpl()

	treeList := []InnerNode{
		c.defaults,
		c.file,
	}

	// TODO: handle all configuration sources
	for _, tree := range treeList {
		err := root.Merge(tree)
		if err != nil {
			return err
		}
	}

	c.root = root
	return nil
}

func (c *ntmConfig) getConfigFile() string {
	if c.configFile == "" {
		return "datadog.yaml"
	}
	return c.configFile
}

// ReadInConfig wraps Viper for concurrent access
func (c *ntmConfig) ReadInConfig() error {
	c.Lock()
	defer c.Unlock()

	err := c.readInConfig(c.getConfigFile())
	if err != nil {
		return err
	}

	for _, f := range c.extraConfigFilePaths {
		err = c.readInConfig(f)
		if err != nil {
			return err
		}
	}
	return c.mergeAllLayers()
}

// ReadConfig wraps Viper for concurrent access
func (c *ntmConfig) ReadConfig(in io.Reader) error {
	c.Lock()
	defer c.Unlock()

	content, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	if err := c.readConfigurationContent(content); err != nil {
		return err
	}
	return c.mergeAllLayers()
}

func (c *ntmConfig) readInConfig(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return c.readConfigurationContent(content)
}

func (c *ntmConfig) readConfigurationContent(content []byte) error {
	var obj map[string]interface{}
	if err := yaml.Unmarshal(content, &obj); err != nil {
		return err
	}
	c.warnings = append(c.warnings, loadYamlInto(c.defaults, c.file, obj, "")...)
	// Mark the config as ready
	c.ready.Store(true)
	return nil
}

// toMapStringInterface convert any type of map into a map[string]interface{}
func toMapStringInterface(data any, path string) (map[string]interface{}, error) {
	if res, ok := data.(map[string]interface{}); ok {
		return res, nil
	}

	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Map:
		convert := map[string]interface{}{}
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			switch k := key.Interface().(type) {
			case string:
				convert[k] = iter.Value().Interface()
			default:
				return nil, fmt.Errorf("error non-string key type for map for '%s'", path)
			}
		}
		return convert, nil
	}
	return nil, fmt.Errorf("invalid type from configuration for key '%s'", path)
}

// loadYamlInto fetch the value for known setings and set them in a tree. The function returns a list of warning about
// unknown settings or invalid types from the YAML.
//
// The function traverses a object loaded from YAML, checking if each node is known within the configuration.
// If known, the value from the YAML blob is imported into the 'dest' tree. If unknown, a warning will be created.
func loadYamlInto(defaults InnerNode, dest InnerNode, data map[string]interface{}, path string) []string {
	if path != "" {
		path = path + "."
	}

	warnings := []string{}
	for key, value := range data {
		key = strings.ToLower(key)
		curPath := path + key

		// check if the key is know in the defaults
		defaultNode, err := defaults.GetChild(key)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("unknown key from YAML: %s", curPath))
			continue
		}

		// if the default is a leaf we create a new leaf in dest
		if _, isLeaf := defaultNode.(LeafNode); isLeaf {
			// check that dest don't have a inner leaf under that name
			c, _ := dest.GetChild(key)
			if _, ok := c.(InnerNode); ok {
				// Both default and dest have a child but they conflict in type. This should never happen.
				warnings = append(warnings, "invalid tree: default and dest tree don't have the same layout")
			} else {
				dest.InsertChildNode(key, newLeafNodeImpl(value, model.SourceFile))
			}
			continue
		}

		mapString, err := toMapStringInterface(value, curPath)
		if err != nil {
			warnings = append(warnings, err.Error())
		}

		// by now we know defaultNode is an InnerNode
		defaultNext, _ := defaultNode.(InnerNode)

		if !dest.HasChild(key) {
			destInner := newInnerNodeImpl()
			warnings = append(warnings, loadYamlInto(defaultNext, destInner, mapString, curPath)...)
			dest.InsertChildNode(key, destInner)
			continue
		}

		child, _ := dest.GetChild(key)
		destChildInner, ok := child.(InnerNode)
		if !ok {
			// Both default and dest have a child but they conflict in type. This should never happen.
			warnings = append(warnings, "invalid tree: default and dest tree don't have the same layout")
			continue
		}
		warnings = append(warnings, loadYamlInto(defaultNext, destChildInner, mapString, curPath)...)
	}
	return warnings
}
