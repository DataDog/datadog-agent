// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"text/template"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
)

func TestConfig(t *testing.T) {
	tmpl, err := template.New("test_config").Parse(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	root, err := ioutil.TempDir("", "test-secagent-root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	if err := ioutil.WriteFile(path.Join(root, "default.policy"), []byte(config.DefaultPolicy), 0644); err != nil {
		t.Fatal(err)
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir": root,
	}); err != nil {
		t.Fatal(err)
	}

	aconfig.Datadog.SetConfigType("yaml")
	if err := aconfig.Datadog.ReadConfig(buffer); err != nil {
		t.Fatal(err)
	}

	_, err = module.NewModule(nil)
	if err != nil {
		t.Fatal(err)
	}
}
