// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"fmt"
	"io"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// ManagedChartVersion pins chart defaults and env precedence for explicit mode.
// Legacy installations continue using their existing chart selection policy.
const ManagedChartVersion = "3.245.2"

const ManagedChartSHA256 = "7722489c353711d7c4034ea0d8f20d3b6d58a4730669e4235393c1ab77c336b7"

func validateRenderedRouting(c *chart.Chart, values map[string]interface{}, p receivers.Plan) error {
	if c.Metadata.Version != ManagedChartVersion {
		return fmt.Errorf("explicit routing requires Datadog chart %s", ManagedChartVersion)
	}
	rv, err := chartutil.ToRenderValues(c, values, chartutil.ReleaseOptions{Name: releaseName, Namespace: "datadog", Revision: 1, IsInstall: true}, nil)
	if err != nil {
		return fmt.Errorf("preparing managed chart render")
	}
	manifests, err := engine.Render(c, rv)
	if err != nil {
		return fmt.Errorf("managed chart rendering failed (details withheld to protect credentials)")
	}
	for name, manifest := range manifests {
		if strings.HasSuffix(name, "NOTES.txt") {
			continue
		}
		dec := yaml.NewDecoder(strings.NewReader(manifest))
		for {
			var doc map[string]interface{}
			err := dec.Decode(&doc)
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("invalid rendered chart YAML")
			}
			if err := validatePodEnvs(doc, p); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

func validatePodEnvs(node any, p receivers.Plan) error {
	switch n := node.(type) {
	case map[string]interface{}:
		for key, v := range n {
			if key == "containers" {
				containers, ok := v.([]interface{})
				if !ok {
					continue
				}
				for _, raw := range containers {
					c, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					envs, _ := c["env"].([]interface{})
					seen := map[string]bool{}
					hasAPIKey := false
					for _, raw := range envs {
						env, ok := raw.(map[string]interface{})
						if !ok {
							continue
						}
						name, _ := env["name"].(string)
						if seen[name] {
							return fmt.Errorf("duplicate rendered env %s", name)
						}
						seen[name] = true
						if name == "DD_API_KEY" {
							hasAPIKey = true
							continue
						}
						k := receivers.EnvKey(name)
						value := env["value"]
						if value == "false" {
							value = false
						}
						if receivers.Unsupported(k, value) {
							return fmt.Errorf("unsupported rendered backend setting %s", name)
						}
						if k == "remote_configuration.enabled" || name == "DD_REMOTE_CONFIGURATION_ENABLED" {
							if p.RemoteConfig == "disabled" && value != false {
								return fmt.Errorf("RC must be disabled")
							}
							continue
						}
						if receivers.Owns(k) {
							want, ok := p.Settings()[k]
							// EnvKey cannot unambiguously reverse nested underscores. Match the
							// forward source mapping instead (e.g. APM telemetry.dd_url).
							if !ok {
								for setting, w := range p.Settings() {
									if receivers.EnvName(setting) == name {
										want, ok = w, true
									}
								}
							}
							if !ok || fmt.Sprint(env["value"]) != fmt.Sprint(want) {
								return fmt.Errorf("rendered %s conflicts with receiver", name)
							}
						}
					}
					if hasAPIKey && p.Endpoint != "" {
						for k := range p.Settings() {
							if !seen[receivers.EnvName(k)] {
								return fmt.Errorf("managed producer lacks %s", receivers.EnvName(k))
							}
						}
					}
				}
			} else {
				if err := validatePodEnvs(v, p); err != nil {
					return err
				}
			}
		}
	case []interface{}:
		for _, item := range n {
			if err := validatePodEnvs(item, p); err != nil {
				return err
			}
		}
	}
	return nil
}
