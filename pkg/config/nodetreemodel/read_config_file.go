// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

func (c *ntmConfig) findConfigFile() {
	if c.configFile == "" {
		for _, path := range c.configPaths {
			configFilePath := filepath.Join(path, c.configName+".yaml")
			if _, err := os.Stat(configFilePath); err == nil {
				c.configFile = configFilePath
				return
			}
		}
	}
}

// ReadInConfig wraps Viper for concurrent access
func (c *ntmConfig) ReadInConfig() error {
	if !c.isReady() {
		return log.Errorf("attempt to ReadInConfig before config is constructed")
	}

	c.Lock()
	defer c.Unlock()

	c.findConfigFile()
	err := c.readInConfig(c.configFile)
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
	if !c.isReady() {
		return log.Errorf("attempt to ReadConfig before config is constructed")
	}

	c.Lock()
	defer c.Unlock()

	content, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	if err := c.readConfigurationContent(c.file, model.SourceFile, content); err != nil {
		return err
	}
	return c.mergeAllLayers()
}

func (c *ntmConfig) readInConfig(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return c.readConfigurationContent(c.file, model.SourceFile, content)
}

func (c *ntmConfig) readConfigurationContent(target InnerNode, source model.Source, content []byte) error {
	var inData map[string]interface{}

	if strictErr := yaml.UnmarshalStrict(content, &inData); strictErr != nil {
		log.Errorf("warning reading config file: %v\n", strictErr)
		if err := yaml.Unmarshal(content, &inData); err != nil {
			return err
		}
	}
	c.warnings = append(c.warnings, loadYamlInto(target, source, inData, "", c.schema)...)
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

// loadYamlInto traverses input data parsed from YAML, checking if each node is defined by the schema.
// If found, the value from the YAML blob is imported into the 'dest' tree. Otherwise, a warning will be created.
func loadYamlInto(dest InnerNode, source model.Source, inData map[string]interface{}, atPath string, schema InnerNode) []string {
	warnings := []string{}
	for key, value := range inData {
		key = strings.ToLower(key)
		currPath := joinKey(atPath, key)

		// check if the key is defined in the schema
		schemaChild, err := schema.GetChild(key)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("unknown key from YAML: %s", currPath))
			continue
		}

		// if the node in the schema is a leaf, then we create a new leaf in dest
		if _, isLeaf := schemaChild.(LeafNode); isLeaf {
			// check that dest doesn't have a inner leaf under that name
			c, _ := dest.GetChild(key)
			if _, ok := c.(InnerNode); ok {
				// Both default and dest have a child but they conflict in type. This should never happen.
				warnings = append(warnings, "invalid tree: default and dest tree don't have the same layout")
			} else {
				dest.InsertChildNode(key, newLeafNode(value, source))
			}
			continue
		}
		// by now we know schemaNode is an InnerNode
		schemaInner, _ := schemaChild.(InnerNode)

		childValue, err := toMapStringInterface(value, currPath)
		if err != nil {
			warnings = append(warnings, err.Error())
		}

		if !dest.HasChild(key) {
			destChildInner := newInnerNode(nil)
			warnings = append(warnings, loadYamlInto(destChildInner, source, childValue, currPath, schemaInner)...)
			dest.InsertChildNode(key, destChildInner)
			continue
		}

		destChild, _ := dest.GetChild(key)
		destChildInner, ok := destChild.(InnerNode)
		if !ok {
			// Both default and dest have a child but they conflict in type. This should never happen.
			warnings = append(warnings, "invalid tree: default and dest tree don't have the same layout")
			continue
		}
		warnings = append(warnings, loadYamlInto(destChildInner, source, childValue, currPath, schemaInner)...)
	}
	return warnings
}
