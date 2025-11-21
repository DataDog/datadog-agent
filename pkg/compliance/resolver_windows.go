// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/sys/windows/registry"
	yamlv2 "gopkg.in/yaml.v2"
	yamlv3 "gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// inputsResolveTimeout is the timeout that is applied for inputs resolution of one
// Rule.
const inputsResolveTimeout = 5 * time.Second

// ResolverOptions is an options struct required to instantiate a Resolver
// instance.
type ResolverOptions struct {
	// Hostname is the name of the host running the resolver.
	Hostname string

	// HostRoot is the path to the mountpoint of host root filesystem. In case
	// the compliance module is run as part of a container.
	HostRoot string

	// HostRootPID sets the resolving context relative to a specific process
	// ID (optional)
	HostRootPID int32

	// StatsdClient is the statsd client used internally by the compliance
	// resolver (optional)
	StatsdClient statsd.ClientInterface
}

// Resolver interface defines a generic method to resolve the inputs
// associated with a given rule. The Close() method should be called whenever
// the resolver is stopped being used to cleanup underlying resources.
type Resolver interface {
	ResolveInputs(ctx context.Context, rule *Rule) (ResolvedInputs, error)
	Close()
}

type windowsResolver struct {
	opts ResolverOptions
}

type fileData struct {
	path string
	data []byte
}

// NewResolver returns the default inputs resolver that is able to resolve any
// kind of supported inputs. It holds a small cache for loaded file metadata
// and different client connexions that may be used for inputs resolution.
func NewResolver(ctx context.Context, opts ResolverOptions) Resolver {
	r := &windowsResolver{
		opts: opts,
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r
}

func (r *windowsResolver) Close() {
}

// ResolveInputs extracts data requested by the input specs of the given rule and normalizes
// the data with predefined schemas that the rego rule consumes. The input schemas must be
// agreed beforehand between the rego content authors and the Go resolver.
func (r *windowsResolver) ResolveInputs(ctx context.Context, rule *Rule) (ResolvedInputs, error) {
	resolvingContext := ResolvingContext{
		RuleID:     rule.ID,
		Hostname:   r.opts.Hostname,
		InputSpecs: make(map[string]*InputSpec),
	}

	if len(rule.InputSpecs) == 0 {
		return nil, fmt.Errorf("no inputs for rule %s", rule.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, inputsResolveTimeout)
	defer cancel()

	rootPath := r.opts.HostRoot

	resolved := make(map[string]interface{})
	for _, spec := range rule.InputSpecs {
		start := time.Now()

		var err error
		var resultType string
		var result interface{}

		// Extract data for the input
		switch {
		case spec.Registry != nil:
			resultType = "registry"
			result, err = r.resolveRegistry(ctx, *spec.Registry)
		case spec.File != nil:
			resultType = "file"
			result, err = r.resolveFile(ctx, rootPath, *spec.File)
		case spec.Constants != nil:
			resultType = "constants"
			result = *spec.Constants
		default:
			return nil, fmt.Errorf("bad input spec")
		}

		tagName := resultType
		if spec.TagName != "" {
			tagName = spec.TagName
		}
		if err != nil {
			return nil, fmt.Errorf("could not resolve input spec %s(tagged=%q): %w", resultType, tagName, err)
		}

		if _, ok := resolved[tagName]; ok {
			return nil, fmt.Errorf("input with tag %q already set", tagName)
		}
		if _, ok := resolvingContext.InputSpecs[tagName]; ok {
			return nil, fmt.Errorf("input with tag %q already set", tagName)
		}

		resolvingContext.InputSpecs[tagName] = spec

		if r, ok := result.([]interface{}); ok && reflect.ValueOf(r).IsNil() {
			result = nil
		}
		if result != nil {
			resolved[tagName] = result
		}

		if statsdClient := r.opts.StatsdClient; statsdClient != nil && resultType != "constants" {
			tags := []string{
				"rule_id:" + rule.ID,
				"rule_input_type:" + resultType,
				"agent_version:" + version.AgentVersion,
			}
			if err := statsdClient.Count(metrics.MetricInputsHits, 1, tags, 1.0); err != nil {
				log.Errorf("failed to send input metric: %v", err)
			}
			if err := statsdClient.Timing(metrics.MetricInputsDuration, time.Since(start), tags, 1.0); err != nil {
				log.Errorf("failed to send input metric: %v", err)
			}
		}
	}

	return NewResolvedInputs(resolvingContext, resolved)
}

func (r *windowsResolver) pathNormalize(rootPath, path string) string {
	if rootPath != "" {
		return filepath.Join(rootPath, path)
	}
	return path
}

func (r *windowsResolver) pathRelative(rootPath, path string) string {
	if rootPath != "" {
		p, err := filepath.Rel(rootPath, path)
		if err != nil {
			return path
		}
		return string(os.PathSeparator) + p
	}
	return path
}

func (r *windowsResolver) pathNormalizeToHostRoot(path string) string {
	return r.pathNormalize(r.opts.HostRoot, path)
}

// getFileData reads the raw data from a file.
func (r *windowsResolver) getFileData(path string) (*fileData, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var data []byte

	if !info.IsDir() {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	return &fileData{
		path: path,
		data: data,
	}, nil
}

// resolveRegistry extracts data from the registry according to the input spec.
func (r *windowsResolver) resolveRegistry(ctx context.Context, spec InputSpecRegistry) (result interface{}, err error) {
	var regHive registry.Key
	var s string
	var m []string
	var i uint64

	hive := strings.TrimSpace(spec.Hive)
	keyPath := strings.TrimSpace(spec.Path)
	valueName := strings.TrimSpace(spec.ValueName)
	valueType := strings.TrimSpace(spec.ValueType)
	values := []string{}

	log.Debugf("resolving registry input: %s\\%s, value name: %s, value type: %s", hive, keyPath, valueName, valueType)

	switch hive {
	case "HKLM":
		regHive = registry.LOCAL_MACHINE
	case "HKCU":
		regHive = registry.CURRENT_USER
	case "HKCR":
		regHive = registry.CLASSES_ROOT
	case "HKCC":
		regHive = registry.CURRENT_CONFIG
	case "HKU":
		regHive = registry.USERS
	default:
		return nil, fmt.Errorf("unsupported registry hive: %s", spec.Hive)
	}

	k, err := registry.OpenKey(regHive, keyPath, registry.QUERY_VALUE)
	if err != nil {
		log.Errorf("failed to open registry: %v", err)

		if errors.Is(err, os.ErrPermission) ||
			errors.Is(err, os.ErrNotExist) ||
			errors.Is(err, os.ErrClosed) {
			return nil, nil
		}

		return nil, err
	}
	defer k.Close()

	switch valueType {
	case "REG_SZ":
		s, _, err = k.GetStringValue(valueName)
		values = append(values, s)
	case "REG_EXPAND_SZ":
		if s, _, err = k.GetStringValue(valueName); err == nil {
			values = append(values, os.ExpandEnv(s))
		}
	case "REG_MULTI_SZ":
		if m, _, err = k.GetStringsValue(valueName); err == nil {
			values = append(values, m...)
		}
	case "REG_DWORD":
		fallthrough
	case "REG_QWORD":
		if i, _, err = k.GetIntegerValue(valueName); err == nil {
			values = append(values, fmt.Sprintf("%d", i))
		}
	case "REG_NONE":
		break
	default:
		err = fmt.Errorf("unsupported registry value type: %s", valueType)
	}

	if err != nil {
		log.Errorf("failed to query registry value: %v", err)
		return nil, err
	}

	return map[string]interface{}{
		"hive":      hive,
		"path":      keyPath,
		"valueName": valueName,
		"valueType": valueType,
		"values":    values,
	}, nil
}

// resolveFile searches and extracts data from a file according to the input spec.
func (r *windowsResolver) resolveFile(ctx context.Context, rootPath string, spec InputSpecFile) (result interface{}, err error) {
	path := r.pathNormalize(rootPath, strings.TrimSpace(spec.Path))

	log.Debugf("resolving file input: %s", path)

	result, err = r.resolveFilePath(ctx, rootPath, path, spec.Parser)

	if errors.Is(err, os.ErrPermission) ||
		errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, os.ErrClosed) {
		result, err = nil, nil
	}
	return
}

// resolveFile extracts data from a file according to the input spec.
func (r *windowsResolver) resolveFilePath(_ context.Context, rootPath, path, parser string) (interface{}, error) {
	path = r.pathNormalize(rootPath, path)

	// Try read the raw file data.
	file, err := r.getFileData(path)
	if err != nil {
		return nil, err
	}

	// Apply a structure to the file data.
	var content interface{}
	if len(file.data) > 0 {
		switch parser {
		case "yaml":
			err = yamlv3.Unmarshal(file.data, &content)
			if err != nil {
				err = yamlv2.Unmarshal(file.data, &content)
			}
			if err == nil {
				content = jsonquery.NormalizeYAMLForGoJQ(content)
			}
		case "json":
			err = json.Unmarshal(file.data, &content)
		case "raw":
			content = string(file.data)
		default:
			content = ""
		}
		if err != nil {
			return nil, err
		}
	}

	// Rego utilities expect Unix-related fields.
	return map[string]interface{}{
		"path":        r.pathRelative(rootPath, path),
		"permissions": "",
		"user":        "",
		"group":       "",
		"content":     content,
	}, nil
}
