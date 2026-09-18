// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloads

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/workloads/catalog"
	"strings"
	"testing"
)

func TestWorkloadTemplatesUseProducerEndpointAndLazyCredentials(t *testing.T) {
	entry := envstore.Entry{Name: "test", Dir: t.TempDir(), Meta: envstore.Meta{FakeIntakeURL: "http://127.0.0.1:18080"}}
	if _, err := templateVarsFor(entry, "no endpoint or credential required"); err != nil {
		t.Fatal(err)
	}
	if _, err := templateVarsFor(entry, "{{FAKEINTAKE_URL}}"); err == nil {
		t.Fatal("missing fixture accepted")
	}
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{"fakeIntake": []byte(`{"agentURL":"http://192.0.2.10:18080","queryURL":"http://127.0.0.1:18080"}`)}, nil); err != nil {
		t.Fatal(err)
	}
	manifests, err := catalog.Manifests("dogstatsd-standalone-capture", "kind", "")
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Join(manifests, "\n")
	vars, err := templateVarsFor(entry, raw)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderTemplate(raw, vars)
	if strings.Contains(rendered, "127.0.0.1") || strings.Contains(rendered, "DD_ADDITIONAL_ENDPOINTS") || strings.Contains(rendered, "{{") || !strings.Contains(rendered, receivers.DummyAPIKey) || !strings.Contains(rendered, "DD_DD_URL") || !strings.Contains(rendered, "http://192.0.2.10:18080") {
		t.Fatal("incorrect independent capture workload")
	}
	legacy, _ := catalog.Manifests("dogstatsd-standalone", "kind", "")
	if !strings.Contains(strings.Join(legacy, "\n"), "DD_ADDITIONAL_ENDPOINTS") {
		t.Fatal("legacy workload migrated silently")
	}
}
