// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Cluster bases (kind, eks) export their kubeconfig through the
// "kubernetesCluster" snapshot resource. connect writes it to the environment
// directory with its context renamed to the environment name — so several
// environments never collide — merges it into the user's kubeconfig without
// touching unrelated entries, and leaves the current context alone.

// connectCluster configures kubectl access to a cluster environment.
func connectCluster(entry envstore.Entry, printOnly bool) error {
	var cluster outputs.ClusterOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "kubernetesCluster", &cluster); err != nil {
		return fmt.Errorf("reading the cluster snapshot: %w", err)
	}
	cfg, err := renameKubeContext(cluster.KubeConfig, entry.Name)
	if err != nil {
		return err
	}
	dest, err := kubeconfigDestination()
	if err != nil {
		return err
	}

	if printOnly {
		fmt.Printf("would write the kubeconfig (context %q) to %s\n", entry.Name, entry.KubeconfigPath())
		fmt.Printf("would add the %q context to %s (existing entries kept, current context unchanged)\n", entry.Name, dest)
		fmt.Printf("then: kubectl config use-context %s\n", entry.Name)
		return nil
	}

	data, err := clientcmd.Write(*cfg)
	if err != nil {
		return fmt.Errorf("rendering the kubeconfig: %w", err)
	}
	if err := writeFileAtomic(entry.KubeconfigPath(), data); err != nil {
		return err
	}
	if err := mergeKubeconfig(dest, cfg); err != nil {
		return err
	}
	fmt.Printf("wrote the kubeconfig (context %q) to %s\n", entry.Name, entry.KubeconfigPath())
	fmt.Printf("added the %q context to %s\n", entry.Name, dest)
	fmt.Printf("start using it (the current context is not changed automatically):\n")
	fmt.Printf("  kubectl config use-context %s\n", entry.Name)
	fmt.Printf("  kubectl --context %s get ns\n", entry.Name)
	return nil
}

// renameKubeContext parses a kubeconfig, renames its single context (and
// current-context) to env, and keeps the cluster and user references as they
// are. The cluster bases produce single-context kubeconfigs; anything else is
// an honest error rather than a guess.
func renameKubeContext(kubeconfigYAML, env string) (*api.Config, error) {
	cfg, err := clientcmd.Load([]byte(kubeconfigYAML))
	if err != nil {
		return nil, fmt.Errorf("parsing the kubeconfig: %w", err)
	}
	switch len(cfg.Contexts) {
	case 1:
	case 0:
		return nil, fmt.Errorf("the kubeconfig has no context")
	default:
		return nil, fmt.Errorf("the kubeconfig has %d contexts; connect only handles the single-context kubeconfigs our cluster bases produce", len(cfg.Contexts))
	}
	var old string
	for name := range cfg.Contexts {
		old = name
	}
	cfg.Contexts[env] = cfg.Contexts[old]
	delete(cfg.Contexts, old)
	cfg.CurrentContext = env
	return cfg, nil
}

// kubeconfigDestination is where the merged context is written: the first
// KUBECONFIG entry (where kubectl itself writes), else ~/.kube/config.
func kubeconfigDestination() (string, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		for _, part := range strings.Split(kubeconfig, string(os.PathListSeparator)) {
			if part != "" {
				return part, nil
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kube", "config"), nil
}

// mergeKubeconfig adds src's cluster, user and context entries to the
// kubeconfig at destPath without touching anything else and without changing
// its current context.
func mergeKubeconfig(destPath string, src *api.Config) error {
	dest := api.NewConfig()
	if data, err := os.ReadFile(destPath); err == nil {
		dest, err = clientcmd.Load(data)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", destPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for name, cluster := range src.Clusters {
		dest.Clusters[name] = cluster
	}
	for name, auth := range src.AuthInfos {
		dest.AuthInfos[name] = auth
	}
	for name, context := range src.Contexts {
		dest.Contexts[name] = context
	}
	data, err := clientcmd.Write(*dest)
	if err != nil {
		return err
	}
	return writeFileAtomic(destPath, data)
}

// writeFileAtomic writes data through a temp file in the same directory and a
// rename, so a crash never leaves a half-written file.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".e2ectl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path) // CreateTemp files are 0600
}
