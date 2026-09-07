// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
)

func TestRegisteredStarterConfigs(t *testing.T) {
	for _, id := range IDs() {
		t.Run(id, func(t *testing.T) {
			d, err := Get(id)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(d.Description()) == "" {
				t.Fatal("registered environments need a discovery description")
			}
			data, err := StarterConfig(id)
			if err != nil {
				t.Fatal(err)
			}
			cfg, errs := config.Parse(data)
			if len(errs) != 0 {
				t.Fatalf("starter config must pass normal parsing: %v", errs)
			}
			if cfg.Environment.Base != id {
				t.Fatalf("starter for %q selects %q", id, cfg.Environment.Base)
			}
			if cfg.Agent.APIKey != "" {
				t.Fatal("starter config must not contain credentials")
			}
			if !strings.HasPrefix(string(data), "#") {
				t.Fatal("starter config should retain its explanatory comments")
			}
		})
	}
}

func TestStarterConfigUnknownBase(t *testing.T) {
	_, err := StarterConfig("not-a-registered-environment")
	if err == nil || !strings.Contains(err.Error(), "registered:") {
		t.Fatalf("expected the registered choices in the error, got: %v", err)
	}
}
