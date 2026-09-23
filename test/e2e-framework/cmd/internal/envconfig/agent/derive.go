// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DefaultVersion is the pinned released Agent used when no source is selected
// on kind and ec2-host: the same version the pinned runtime image, the starter
// examples and explicit routing support.
const DefaultVersion = "7.83.0"

// DefaultDevImage is the deterministic development image reference the local
// checkout build produces on kind: never a released tag, never mutable latest.
const DefaultDevImage = "localhost/datadog-agent:7.83.0-e2ectl-dev"

// Build is a derived internal build-provider selection (agent.build's shape,
// never user-writable anymore).
type Build struct {
	Provider string
	Section  []byte
}

// Resolution is the derived internal installation: the installer ID and its
// installer-owned section, plus the optional build-provider selection. The
// existing installer machinery consumes these unchanged.
type Resolution struct {
	Installer string
	Section   []byte
	Build     *Build
}

// Derive maps the simple user selection onto the internal installer and
// build-provider contracts. It is a pure function of (base, agent config)
// except for the source-checkout root, which is the e2ectl invocation root
// (os.Getwd), the same default the legacy binary path used. Unsupported
// combinations are rejected here with actionable messages, never silently
// substituted.
//
// Derivation table (as implemented):
//
//	source              | local       binary / invoke-binary (build from checkout)
//	source              | kind        helm / invoke-image (dev image from checkout)
//	source              | ec2-host    rejected: remote source builds not automated
//	source              | eks         rejected: no remote image delivery yet
//	source              | docker-host rejected: remote source builds not automated
//	pipeline: N         | local       package / pipeline (download the pipeline DEB)
//	pipeline: N         | kind        rejected: pipeline images not downloadable yet
//	pipeline: N         | eks         rejected: pipeline images not downloadable yet
//	pipeline: N         | ec2-host    package / pipeline (download the pipeline DEB)
//	pipeline: N         | docker-host package / pipeline (download the pipeline DEB)
//	version: X           | local       rejected: released versions target kind, eks and host bases
//	version: X           | kind        helm (chart version X)
//	version: X           | eks         helm (chart version X)
//	version: X           | ec2-host    script (install script version X)
//	version: X           | docker-host script (install script version X)
//	omitted              | local       as source: true
//	omitted              | kind        as version: DefaultVersion
//	omitted              | eks         as version: DefaultVersion
//	omitted              | ec2-host    as version: DefaultVersion
//	omitted              | docker-host as version: DefaultVersion
func Derive(base string, c Config) (Resolution, error) {
	if c.Pipeline < 0 {
		return Resolution{}, fmt.Errorf("agent.pipeline: must be a positive pipeline id")
	}
	set := 0
	for _, on := range []bool{c.Source, c.Pipeline != 0, c.Version != ""} {
		if on {
			set++
		}
	}
	if set > 1 {
		return Resolution{}, fmt.Errorf("agent: source, pipeline and version are mutually exclusive: pick exactly one (or none for the environment default)")
	}
	// Helm chart values exist only on the Helm mechanisms (kind, eks).
	if strings.TrimSpace(c.Values) != "" && base != "kind" && base != "eks" {
		return Resolution{}, fmt.Errorf("agent.values: Helm chart values are supported on the Helm bases kind and eks only")
	}
	switch {
	case c.Source:
		return deriveSource(base, c)
	case c.Pipeline != 0:
		return derivePipeline(base, c)
	case c.Version != "":
		return deriveVersion(base, c)
	default:
		switch base {
		case "local":
			return deriveSource(base, c)
		case "kind", "eks", "ec2-host", "docker-host":
			return deriveVersion(base, c)
		}
		return Resolution{}, fmt.Errorf("agent: environment.base %q has no default source (supported: source, pipeline, version)", base)
	}
}

func deriveSource(base string, c Config) (Resolution, error) {
	root, err := os.Getwd()
	if err != nil {
		return Resolution{}, fmt.Errorf("agent.source: resolving the checkout root: %w", err)
	}
	switch base {
	case "local":
		section, err := marshalSection(map[string]any{"config": c.Config, "integrations": c.Integrations})
		if err != nil {
			return Resolution{}, err
		}
		build, err := marshalSection(map[string]any{"repository": root})
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Installer: "binary", Section: section, Build: &Build{Provider: "invoke-binary", Section: build}}, nil
	case "kind":
		values, err := kindValues(c)
		if err != nil {
			return Resolution{}, err
		}
		section, err := marshalSection(map[string]any{"values": values})
		if err != nil {
			return Resolution{}, err
		}
		// The pinned 7.83.0 base image is the invoke-image schema default; the
		// deterministic dev reference keeps local builds out of released tags.
		build, err := marshalSection(map[string]any{"repository": root, "reference": DefaultDevImage})
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Installer: "helm", Section: section, Build: &Build{Provider: "invoke-image", Section: build}}, nil
	case "ec2-host", "docker-host":
		return Resolution{}, fmt.Errorf("agent.source: local source builds are not yet automated for remote targets — use a pipeline or version for base %q", base)
	case "eks":
		return Resolution{}, fmt.Errorf("agent.source: local image builds cannot be delivered to remote clusters yet — use version for a released chart on base %q", base)
	}
	return Resolution{}, fmt.Errorf("agent.source: environment.base %q does not support source builds", base)
}

func derivePipeline(base string, c Config) (Resolution, error) {
	switch base {
	case "local":
		if len(c.Integrations) > 0 {
			return Resolution{}, fmt.Errorf("agent.integrations: not supported on base %q with a pipeline source: the container target runs the fixed core checks", base)
		}
	case "kind":
		return Resolution{}, fmt.Errorf("agent.pipeline: pipeline images are not downloadable yet — use version for a released chart or source: true for a local development image on base %q", base)
	case "eks":
		return Resolution{}, fmt.Errorf("agent.pipeline: pipeline images are not downloadable yet — use version for a released chart on base %q", base)
	case "ec2-host", "docker-host":
	default:
		return Resolution{}, fmt.Errorf("agent.pipeline: environment.base %q does not support pipeline packages", base)
	}
	section, err := marshalSection(map[string]any{"allow-unsigned": true, "config": c.Config, "integrations": c.Integrations})
	if err != nil {
		return Resolution{}, err
	}
	build, err := marshalSection(map[string]any{"pipeline": c.Pipeline})
	if err != nil {
		return Resolution{}, err
	}
	// The sha256 pin is computed and verified by the pipeline download
	// provider over the exact downloaded file; it is never user-typed.
	return Resolution{Installer: "package", Section: section, Build: &Build{Provider: "pipeline", Section: build}}, nil
}

func deriveVersion(base string, c Config) (Resolution, error) {
	version := c.Version
	if version == "" {
		version = DefaultVersion
	}
	switch base {
	case "kind", "eks":
		values, err := kindValues(c)
		if err != nil {
			return Resolution{}, err
		}
		section, err := marshalSection(map[string]any{"version": version, "values": values})
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Installer: "helm", Section: section}, nil
	case "ec2-host", "docker-host":
		section, err := marshalSection(map[string]any{"version": version, "config": c.Config, "integrations": c.Integrations})
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Installer: "script", Section: section}, nil
	case "local":
		return Resolution{}, fmt.Errorf("agent.version: released versions install on the Helm bases (kind, eks) or the host bases (ec2-host, docker-host); for the local base use source: true (build from checkout) or pipeline: N (pipeline DEB in Docker)")
	}
	return Resolution{}, fmt.Errorf("agent.version: environment.base %q does not support released versions", base)
}

// kindValues merges the user's chart values with the conf.d integration
// contents: the chart's datadog.confd is the mechanism-appropriate location
// for integrations on the Helm bases (kind, eks). A raw datadog.yaml blob has no chart location, so
// config is rejected there with a pointer to values.
func kindValues(c Config) (string, error) {
	if strings.TrimSpace(c.Config) != "" {
		return "", fmt.Errorf("agent.config: a datadog.yaml blob has no Helm chart location — express it as chart values under agent.values on base %q", "kind")
	}
	values := map[string]any{}
	if strings.TrimSpace(c.Values) != "" {
		if err := yaml.Unmarshal([]byte(c.Values), &values); err != nil {
			return "", fmt.Errorf("agent.values: not valid YAML chart values")
		}
	}
	if len(c.Integrations) == 0 {
		return c.Values, nil
	}
	datadog, ok := values["datadog"].(map[string]any)
	if !ok {
		datadog = map[string]any{}
		values["datadog"] = datadog
	}
	confd, ok := datadog["confd"].(map[string]any)
	if !ok {
		confd = map[string]any{}
		datadog["confd"] = confd
	}
	for folder, content := range c.Integrations {
		check := strings.TrimSuffix(folder, ".d")
		if check == folder {
			return "", fmt.Errorf("agent.integrations: %q is not a valid conf.d folder name", folder)
		}
		confd[check] = content
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// marshalSection renders an internal installer/build section. Only derived
// values reach this; empty values are omitted so the installer schema's
// defaults and required rules apply unchanged.
func marshalSection(fields map[string]any) ([]byte, error) {
	out := map[string]any{}
	for name, value := range fields {
		switch v := value.(type) {
		case string:
			if v != "" {
				out[name] = v
			}
		case map[string]string:
			if len(v) != 0 {
				out[name] = v
			}
		case bool:
			if v {
				out[name] = v
			}
		default:
			out[name] = value
		}
	}
	return yaml.Marshal(out)
}

// DefaultSource reports the source a base installs when none is selected —
// the value e2ectl environments advertises and init generates.
func DefaultSource(base string) string {
	switch base {
	case "local":
		return "source: true"
	default:
		return "version: " + DefaultVersion
	}
}

// Example returns the annotated starter agent section for a base: its default
// source selection with a commented tour of every configurable field, marked
// with the bases each works on.
func Example(base string) (*yaml.Node, error) {
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
	key := str(DefaultSourceField(base))
	key.HeadComment = strings.Join([]string{
		"Agent source — pick exactly ONE (or none for this base's default):",
		"  source: true       build the Agent from this checkout (local, kind)",
		"  pipeline: 123456   install the CI pipeline's DEB artifacts (local, ec2-host, docker-host)",
		"  version: \"7.x.y\"   install a released agent version (kind, eks, ec2-host, docker-host)",
		"",
		"config: extra datadog.yaml merged over the installer's generated one",
		"  (local and host bases; on the Helm bases kind/eks express it as chart",
		"  values under values -> datadog: instead). Example:",
		"  config: |",
		"    logs_enabled: true",
		"    tags:",
		"      - \"env:e2ectl\"",
		"",
		"integrations: conf.d folder name -> its conf.yaml contents (every base;",
		"  rendered into datadog.confd on the Helm bases kind/eks). Example:",
		"  integrations:",
		"    nginx.d: |",
		"      init_config:",
		"      instances:",
		"        - nginx_status_url: http://%%host%%/nginx_status",
	}, "\n")
	if base == "kind" || base == "eks" {
		key.HeadComment += strings.Join([]string{
			"",
			"",
			"values: extra Helm chart values deep-merged over the installer's defaults",
			"  (kind/eks only) — every knob the Datadog Helm chart exposes works here.",
			"  Example:",
			"  values: |",
			"    datadog:",
			"      logs:",
			"        enabled: true",
			"        containerCollectAll: true",
			"      apm:",
			"        enabled: true",
		}, "\n")
	}
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if base == "local" {
		n.Content = append(n.Content, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	} else {
		n.Content = append(n.Content, key, str(DefaultVersion))
	}
	return n, nil
}

func DefaultSourceField(base string) string {
	if base == "local" {
		return "source"
	}
	return "version"
}
