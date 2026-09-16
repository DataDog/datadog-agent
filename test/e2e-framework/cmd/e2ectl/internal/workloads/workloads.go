// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloads deploys test applications alongside the Agent. The
// environment determines the mechanism: kind uses kubectl via the snapshot's
// kubeconfig, the local container agent uses docker run on the agent's
// network (the same pattern as the fakeintake).
package workloads

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	wc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/workloads"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/workloads/catalog"
)

// Deploy deploys the declared workloads for the given environment base.
// It is called at the end of install, after the agent is running, so the
// first metrics from workload startup are captured.
func Deploy(cfg *config.File, entry envstore.Entry) error {
	decls := cfg.Workloads.Workloads
	if len(decls) == 0 {
		return nil
	}
	switch cfg.Environment.Base {
	case "kind":
		return deployKubernetes(cfg, entry)
	case "local":
		return deployDocker(entry, decls)
	default:
		return fmt.Errorf("workloads are not supported on base %q yet", cfg.Environment.Base)
	}
}

// Validate checks that the workload declarations are compatible with the
// selected environment. Called at prepare time — before infrastructure
// is created.
func Validate(cfg *config.File) []error {
	decls := cfg.Workloads.Workloads
	if len(decls) == 0 {
		return nil
	}
	var errs []error
	for i, w := range decls {
		switch {
		case w.Manifest != "" && cfg.Environment.Base != "kind":
			errs = append(errs, fmt.Errorf(
				"workloads[%d].manifest: Kubernetes manifests are not supported on base %q (supported forms: app, image)",
				i, cfg.Environment.Base))
		case w.App != "" && !catalog.Has(w.App, cfg.Environment.Base):
			available := catalog.AvailableForms(w.App)
			errs = append(errs, fmt.Errorf(
				"workloads[%d].app: %q has no definition for base %q (available: %s)",
				i, w.App, cfg.Environment.Base, strings.Join(available, ", ")))
		}
	}
	return errs
}

// deployKubernetes applies K8s manifests via kubectl (using the kubeconfig
// from the environment's snapshot). The kind driver stores the kubeconfig
// at entry.KubeconfigPath().
func deployKubernetes(cfg *config.File, entry envstore.Entry) error {
	kubeconfig := entry.KubeconfigPath()
	decls := cfg.Workloads.Workloads
	// Ensure target namespaces exist before applying (idempotent).
	namespaces := map[string]bool{}
	for _, w := range decls {
		ns := w.Namespace
		if ns == "" && w.App != "" {
			if entry, ok := catalog.Get(w.App, cfg.Environment.Base); ok && entry.K8sNamespace != "" {
				ns = entry.K8sNamespace
			}
		}
		if ns != "" {
			namespaces[ns] = true
		}
	}
	for ns := range namespaces {
		if err := kubectlEnsureNamespace(kubeconfig, ns); err != nil {
			return err
		}
	}
	for i, w := range decls {
		manifests, err := resolve(w, cfg.Environment.Base)
		if err != nil {
			return fmt.Errorf("workloads[%d]: %w", i, err)
		}
		for _, manifest := range manifests {
			if err := kubectlApply(kubeconfig, manifest, w.Namespace); err != nil {
				return fmt.Errorf("workloads[%d]: %w", i, err)
			}
		}
	}
	return kubectlWait(kubeconfig, cfg.Environment.Base, decls)
}

// deployDocker runs containers on the agent's Docker network — the same
// mechanism as the fakeintake (localinfra.RunFakeintakeOnNetwork).
func deployDocker(entry envstore.Entry, decls []wc.Workload) error {
	network := localinfra.NetworkName(entry.Name)
	for i, w := range decls {
		image, name, err := resolveDocker(w.App, w.Image, w.Name)
		if err != nil {
			return fmt.Errorf("workloads[%d]: %w", i, err)
		}
		containerName := entry.Name + "-" + name
		_ = localinfra.RemoveContainer(containerName) // idempotent re-deploy
		cmd := exec.Command("docker", "run", "-d",
			"--name", containerName,
			"--network", network,
			image,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("workloads[%d]: starting %s: %w", i, name, err)
		}
	}
	return nil
}

// resolve converts a declaration into raw manifest text for the given base.
func resolve(w wc.Workload, base string) ([]string, error) {
	switch {
	case w.App != "":
		return catalog.Manifests(w.App, base, w.Namespace)
	case w.Manifest != "":
		if strings.Contains(w.Manifest, "\n") {
			return []string{w.Manifest}, nil // inline
		}
		data, err := os.ReadFile(w.Manifest)
		if err != nil {
			return nil, fmt.Errorf("reading manifest file %s: %w", w.Manifest, err)
		}
		return splitDocuments(string(data)), nil
	case w.Image != "":
		name := w.Name
		if name == "" {
			name = imageName(w.Image)
		}
		manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: %s
          image: %s
`, name, name, name, name, w.Image)
		return []string{manifest}, nil
	}
	return nil, fmt.Errorf("no workload form set")
}

// resolveDocker converts an app name or image into a Docker run spec.
func resolveDocker(app, image, nameOverride string) (image_, name string, err error) {
	switch {
	case app != "":
		entry, ok := catalog.Get(app, "local")
		if !ok || entry.Image == "" {
			return "", "", fmt.Errorf("app %q has no Docker image for local environments", app)
		}
		return entry.Image, app, nil
	case image != "":
		name := nameOverride
		if name == "" {
			name = imageName(image)
		}
		return image, name, nil
	}
	return "", "", fmt.Errorf("manifest form is not supported on local environments (use app or image)")
}

func imageName(ref string) string {
	s := strings.ReplaceAll(ref, ":", "/")
	parts := strings.Split(s, "/")
	name := parts[len(parts)-1]
	return strings.ReplaceAll(name, "@", "-")
}

func kubectlEnsureNamespace(kubeconfig, namespace string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
		"create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	yaml, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generating namespace %s: %w", namespace, err)
	}
	apply := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	apply.Stdin = bytes.NewReader(yaml)
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("creating namespace %s: %w (%s)", namespace, err, string(out))
	}
	return nil
}

func splitDocuments(data string) []string {
	var docs []string
	for _, doc := range strings.Split(data, "\n---") {
		if strings.TrimSpace(doc) != "" {
			docs = append(docs, doc)
		}
	}
	return docs
}

func kubectlApply(kubeconfig, manifest, namespace string) error {
	args := []string{"apply", "--kubeconfig", kubeconfig, "-f", "-"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(manifest)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

func kubectlWait(kubeconfig, base string, decls []wc.Workload) error {
	for _, w := range decls {
		name := w.Name
		if name == "" {
			if w.App != "" {
				name = w.App
			} else if w.Image != "" {
				name = imageName(w.Image)
			}
		}
		if name == "" {
			continue
		}
		ns := w.Namespace
		if ns == "" && w.App != "" {
			if entry, ok := catalog.Get(w.App, base); ok && entry.K8sNamespace != "" {
				ns = entry.K8sNamespace
			}
		}
		args := []string{"wait", "--kubeconfig", kubeconfig,
			"--for=condition=Available", "deployment/" + name,
			"--timeout=180s"}
		if ns != "" {
			args = append(args, "-n", ns)
		}
		cmd := exec.Command("kubectl", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("waiting for deployment %s: %w", name, err)
		}
	}
	return nil
}
