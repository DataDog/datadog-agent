// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// ReadInConfig resets the file tree and reads the configuration from the file system.
func (c *ntmConfig) ReadInConfig() error {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
		return log.Errorf("attempt to ReadInConfig before config is constructed")
	}

	c.maybeRebuild()

	c.Lock()
	defer c.Unlock()

	// Reset the file tree like Viper does, so previous config is cleared
	c.file = newInnerNode(nil)

	c.findConfigFile()
	if err := c.readInConfig(c.configFile); err != nil {
		return err
	}

	for _, f := range c.extraConfigFilePaths {
		if err := c.readInConfig(f); err != nil {
			return err
		}
	}
	return c.mergeAllLayers()
}

// ReadConfig resets the file tree and reads the configuration from the provided reader.
func (c *ntmConfig) ReadConfig(in io.Reader) error {
	if !c.isReady() && !c.allowDynamicSchema.Load() {
		return log.Errorf("attempt to ReadConfig before config is constructed")
	}

	c.maybeRebuild()

	c.Lock()
	defer c.Unlock()

	// Reset the file tree like Viper does, so previous config is cleared
	c.file = newInnerNode(nil)

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
		return model.NewConfigFileNotFoundError(err) // nolint: forbidigo // constructing proper error
	}
	return c.readConfigurationContent(c.file, model.SourceFile, content)
}

func (c *ntmConfig) readConfigurationContent(target *nodeImpl, source model.Source, content []byte) error {
	var inData map[string]interface{}

	if strictErr := yaml.UnmarshalStrict(content, &inData); strictErr != nil {
		log.Errorf("warning reading config file: %v\n", strictErr)
		if err := yaml.Unmarshal(content, &inData); err != nil {
			return err
		}
	}
	c.warnings = append(c.warnings, loadYamlInto(target, source, inData, "", c.defaults, c.knownKeys, c.unknownKeys)...)
	return nil
}

// buildNestedMap converts keys with dots into a nested structure
// for example:
//
//	buildNestedMap(["a", "b", "c"], 123) => {"a": {"b": {"c": 123}}}
func buildNestedMap(keyParts []string, bottomValue interface{}) map[string]interface{} {
	res := map[string]interface{}{}
	nextKey := keyParts[0]
	if len(keyParts) == 1 {
		res[nextKey] = bottomValue
	} else {
		res[nextKey] = buildNestedMap(keyParts[1:], bottomValue)
	}
	return res
}

var valuelessLeaf = &nodeImpl{}

// loadYamlInto traverses input data parsed from YAML, checking if each node is defined by the schema.
// If found, the value from the YAML blob is imported into the 'dest' tree. Otherwise, a warning will be created.
func loadYamlInto(dest *nodeImpl, source model.Source, inData map[string]interface{}, atPath string, schema *nodeImpl, knownKeys map[string]bool, unknownKeys map[string]struct{}) []error {
	warnings := []error{}
	for key, value := range inData {
		key = strings.ToLower(key)

		// If the key contains a dot, it represents a nested key
		if strings.Contains(key, ".") {
			parts := strings.Split(key, ".")
			key = parts[0]
			value = buildNestedMap(parts[1:], value)
		}
		currPath := joinKey(atPath, key)

		// check if the key is defined in the schema
		schemaChild, err := schema.GetChild(key)
		if err != nil {
			isLeaf, isKnown := knownKeys[currPath]
			if isLeaf {
				// Not found but known, the leaf setting must be valueless (defined by BindEnv or SetKnown)
				schemaChild = valuelessLeaf
			} else {
				if !isKnown {
					warnings = append(warnings, fmt.Errorf("unknown key from YAML: %s", currPath))
				}

				// if the key is not defined in the schema, we can still add it to the destination
				if value == nil || isScalar(value) || isSlice(value) {
					dest.InsertChildNode(key, newLeafNode(value, source))
					unknownKeys[currPath] = struct{}{}
					continue
				}

				// fallback to inner node if it's not a scalar or nil
				schemaChild = newInnerNode(nil)
			}
		}

		// if the node in the schema is a leaf, then we create a new leaf in dest
		if schemaChild.IsLeafNode() {
			// check that dest doesn't have a inner leaf under that name
			c, _ := dest.GetChild(key)
			if c != nil && c.IsInnerNode() {
				// Both default and dest have a child but they conflict in type. This should never happen.
				warnings = append(warnings, errors.New("invalid tree: default and dest tree don't have the same layout"))
			} else {
				dest.InsertChildNode(key, newLeafNode(value, source))
			}
			continue
		}

		childValue, err := ToMapStringInterface(value, currPath)
		if err != nil {
			warnings = append(warnings, err)
			// Insert child node here as a leaf. It has the wrong type, but this maintains better
			// compatibility with how viper works.
			dest.InsertChildNode(key, newLeafNode(value, source))
			continue
		}

		if !dest.HasChild(key) {
			destChild := newInnerNode(nil)
			warnings = append(warnings, loadYamlInto(destChild, source, childValue, currPath, schemaChild, knownKeys, unknownKeys)...)
			dest.InsertChildNode(key, destChild)
			continue
		}

		destChild, _ := dest.GetChild(key)
		if destChild.IsLeafNode() {
			// Both default and dest have a child but they conflict in type. This should never happen.
			warnings = append(warnings, errors.New("invalid tree: default and dest tree don't have the same layout"))
			continue
		}
		warnings = append(warnings, loadYamlInto(destChild, source, childValue, currPath, schemaChild, knownKeys, unknownKeys)...)
	}
	return warnings
}
