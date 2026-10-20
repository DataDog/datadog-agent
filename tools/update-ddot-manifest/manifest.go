// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// distVersionKey is the synthetic change label used for the dist.version field.
const distVersionKey = "dist.version"

// Module path prefixes used to classify components. Core and contrib are
// bumped from the compatibility table; datadog modules are known-but-untouched;
// anything else is treated as a custom component.
const (
	corePrefix    = "go.opentelemetry.io/collector"
	contribPrefix = "github.com/open-telemetry/opentelemetry-collector-contrib"
	datadogPrefix = "github.com/DataDog/"
)

// componentCategories are the manifest top-level keys whose entries carry a
// "gomod: <module> <version>" scalar.
var componentCategories = []string{
	"receivers",
	"processors",
	"exporters",
	"connectors",
	"extensions",
	"providers",
}

// change records a single version rewrite for reporting.
type change struct {
	module string
	oldV   string
	newV   string
}

// updateResult summarizes what updateManifest did.
type updateResult struct {
	changed []change // version rewrites that were applied
	custom  []string // custom module paths detected and left untouched
}

// updateManifest rewrites the version of every known (core/contrib) component
// in an OCB manifest according to v, updates dist.version, and reports any
// custom component left untouched.
//
// The change set is computed by parsing the YAML (robust against the folded
// "gomod" scalars), but the changes are applied as targeted text edits on the
// original bytes so that formatting, comments and the replaces section are
// preserved byte-for-byte — only version tokens change.
func updateManifest(data []byte, v ddotVersion) ([]byte, *updateResult, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil, fmt.Errorf("manifest is not a valid YAML document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("manifest root is not a mapping")
	}

	res := &updateResult{}

	// dist.version -> OCBVersion.
	if dist := mappingValue(root, "dist"); dist != nil {
		if ver := mappingValue(dist, "version"); ver != nil && ver.Kind == yaml.ScalarNode {
			if ver.Value != v.OCBVersion {
				res.changed = append(res.changed, change{module: distVersionKey, oldV: ver.Value, newV: v.OCBVersion})
			}
		}
	}

	// Each component category.
	for _, cat := range componentCategories {
		seq := mappingValue(root, cat)
		if seq == nil || seq.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range seq.Content {
			gomod := mappingValue(item, "gomod")
			if gomod == nil || gomod.Kind != yaml.ScalarNode {
				continue
			}
			// The value is "<module> <version>"; anything else is left as-is.
			fields := strings.Fields(gomod.Value)
			if len(fields) != 2 {
				continue
			}
			module, curVer := fields[0], fields[1]

			newVer, known := newVersionFor(module, curVer, v)
			if !known {
				if !strings.HasPrefix(module, datadogPrefix) {
					res.custom = append(res.custom, module)
				}
				continue
			}
			if newVer != curVer {
				res.changed = append(res.changed, change{module: module, oldV: curVer, newV: newVer})
			}
		}
	}

	return applyChanges(data, res.changed), res, nil
}

// applyChanges rewrites each version token in the raw manifest bytes. Each edit
// is anchored on the (unique) module path — or the "version:" key for
// dist.version — so only the intended token changes and all whitespace,
// line-folding and comments are left intact.
func applyChanges(data []byte, changes []change) []byte {
	out := data
	for _, c := range changes {
		var anchor string
		if c.module == distVersionKey {
			anchor = `(version:\s+)`
		} else {
			anchor = `(` + regexp.QuoteMeta(c.module) + `\s+)`
		}
		re := regexp.MustCompile(anchor + regexp.QuoteMeta(c.oldV) + `\b`)
		out = re.ReplaceAll(out, []byte("${1}"+c.newV))
	}
	return out
}

// newVersionFor returns the target version for a component and whether it is a
// known (core/contrib) component that should be bumped. Datadog and custom
// modules return known=false and are left untouched by the caller.
//
// The core/contrib split follows the current version stream: a component
// already on v1.x is stable and moves to CoreStableVersion; everything else
// (v0.x core beta and all contrib) moves to ContribVersion.
func newVersionFor(module, curVer string, v ddotVersion) (string, bool) {
	if !strings.HasPrefix(module, corePrefix) && !strings.HasPrefix(module, contribPrefix) {
		return "", false
	}
	if strings.HasPrefix(curVer, "v1.") {
		return v.CoreStableVersion, true
	}
	return v.ContribVersion, true
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
