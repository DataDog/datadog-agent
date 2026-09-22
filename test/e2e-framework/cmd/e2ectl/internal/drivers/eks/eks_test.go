// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestDriverRegistrationWiring(t *testing.T) {
	d := New()
	if d.ID() != workerclient.BaseEKS {
		t.Fatalf("driver ID %q must be the shared base spelling %q", d.ID(), workerclient.BaseEKS)
	}
	if d.Description() == "" {
		t.Fatal("the registry requires a description")
	}
	installers := d.Installers()
	if len(installers) != 1 || installers[0].ID() != "helm" {
		t.Fatal("EKS installs through the shared Helm installer only")
	}
}

func TestExportKubeconfig(t *testing.T) {
	t.Run("writes the snapshot kubeconfig", func(t *testing.T) {
		entry := envstore.Entry{Name: "eks", Dir: t.TempDir()}
		if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{
			"kubernetesCluster": []byte(`{"clusterName":"eks","kubeConfig":"apiVersion: v1"}`),
		}, nil); err != nil {
			t.Fatal(err)
		}
		if err := exportKubeconfig(entry, &entry.Meta); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(entry.KubeconfigPath())
		if err != nil || string(data) != "apiVersion: v1" {
			t.Fatalf("kubeconfig not exported verbatim: %q %v", data, err)
		}
	})
	t.Run("rejects an incomplete snapshot", func(t *testing.T) {
		entry := envstore.Entry{Name: "eks", Dir: t.TempDir()}
		if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{
			"kubernetesCluster": []byte(`{"clusterName":"eks"}`),
		}, nil); err != nil {
			t.Fatal(err)
		}
		if err := exportKubeconfig(entry, &entry.Meta); err == nil {
			t.Fatal("a missing kubeconfig must fail the start, not produce a broken file")
		}
		if _, err := os.Stat(entry.KubeconfigPath()); !os.IsNotExist(err) {
			t.Fatal("a failed export must not leave a kubeconfig behind")
		}
	})
}
