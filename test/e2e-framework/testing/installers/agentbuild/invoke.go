// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateRequest(r Request) error {
	if err := r.Target.Native(); err != nil {
		return err
	}
	for _, p := range []string{r.Repository, r.OutputDir} {
		if !filepath.IsAbs(p) || strings.ContainsAny(p, ",:\n\r") {
			return fmt.Errorf("build paths must be absolute and mount-safe")
		}
	}
	if _, err := os.Stat(filepath.Join(r.Repository, "tasks", "agent.py")); err != nil {
		return fmt.Errorf("repository root required: %w", err)
	}
	return nil
}

// Tasks share worktree outputs. Fail concurrent preparation rather than mixing
// two builds. The lock spans staging, not just compilation; stale locks require
// operator inspection/removal, never an unsafe automatic takeover.
func lock(r Request) (func(), error) {
	path := filepath.Join(r.Repository, ".e2ectl-artifact-build.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("build checkout locked (%s): %w", path, err)
	}
	f.Close()
	return func() { os.Remove(path) }, nil
}
func work(r Request) (string, error) {
	if err := os.MkdirAll(r.OutputDir, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(r.OutputDir, "prepare-")
}
func (a Adapter) BuildBinary(ctx context.Context, r BinaryRequest) (Result, error) {
	if err := validateRequest(r.Request); err != nil {
		return Result{}, err
	}
	unlock, err := lock(r.Request)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	dir, err := work(r.Request)
	if err != nil {
		return Result{}, err
	}
	manifest := filepath.Join(dir, "producer.json")
	args := []string{"inv", "agent.build", "--build-exclude=systemd", "--result-manifest=" + manifest}
	if r.Race {
		args = append(args, "--race")
	}
	if _, err = a.runBuild(ctx, r.Repository, "dda", args...); err != nil {
		return Result{}, err
	}
	result, err := Read(manifest)
	if err != nil {
		return result, err
	}
	if result.Binary == nil {
		return result, fmt.Errorf("binary producer returned wrong format")
	}
	if err = result.Validate(r.Target); err != nil {
		return result, err
	}
	return Stage(result, r.OutputDir)
}
func (a Adapter) InspectImage(ctx context.Context, ref string, t Target) (Image, error) {
	if !imagePattern.MatchString(ref) {
		if err := ValidateReference(ref); err != nil {
			return Image{}, err
		}
	}
	if err := t.Validate(); err != nil {
		return Image{}, err
	}
	out, err := a.run(ctx, "", "docker", "image", "inspect", ref)
	if err != nil {
		return Image{}, err
	}
	var images []struct {
		ID          string `json:"Id"`
		OS          string `json:"Os"`
		Arch        string `json:"Architecture"`
		RepoDigests []string
	}
	if err = json.Unmarshal(out, &images); err != nil {
		return Image{}, err
	}
	if len(images) != 1 || images[0].OS != t.OS || images[0].Arch != t.Arch || !imagePattern.MatchString(images[0].ID) {
		return Image{}, fmt.Errorf("image identity/platform mismatch")
	}
	i := Image{Reference: ref, ID: images[0].ID}
	for _, d := range images[0].RepoDigests {
		if strings.HasPrefix(d, strings.Split(ref, "@")[0]+"@") {
			i.RepositoryDigest = d
		}
	}
	return i, nil
}

// ExistingImage never builds, pulls or silently repins a missing receipt.
func (a Adapter) ExistingImage(ctx context.Context, ref, manifest string, t Target) (Result, error) {
	i, err := a.InspectImage(ctx, ref, t)
	if err != nil {
		return Result{}, err
	}
	r := Result{Schema: 1, Target: t, Provenance: Provenance{Producer: "existing-image"}, Image: &i}
	if manifest != "" {
		r, err = Read(manifest)
		if err != nil {
			return r, err
		}
		if r.Image == nil || r.Image.ID != i.ID {
			return r, fmt.Errorf("existing image does not match receipt")
		}
		if err = r.Validate(t); err != nil {
			return r, err
		}
		r.Image.Reference = ref
	}
	return r, r.Validate(t)
}
func (a Adapter) BuildImage(ctx context.Context, r ImageRequest) (Result, error) {
	if err := validateRequest(r.Request); err != nil {
		return Result{}, err
	}
	for _, ref := range []string{r.Reference, r.BaseImage} {
		if err := ValidateReference(ref); err != nil {
			return Result{}, err
		}
	}
	args := []string{"inv", "agent.hacky-dev-image-build", "--target-image=" + r.Reference, "--base-image=" + r.BaseImage, "--arch=" + r.Target.Arch}
	seen := map[string]bool{}
	for _, c := range r.RebuildComponents {
		if seen[c] {
			return Result{}, fmt.Errorf("duplicate rebuilt component %s", c)
		}
		seen[c] = true
		switch c {
		case "agent":
		case "trace-agent", "process-agent", "security-agent", "system-probe", "trace-loader", "privateactionrunner":
			args = append(args, "--"+c)
		default:
			return Result{}, fmt.Errorf("unsupported rebuilt component %s", c)
		}
	}
	if r.Race {
		args = append(args, "--race")
	}
	unlock, err := lock(r.Request)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	dir, err := work(r.Request)
	if err != nil {
		return Result{}, err
	}
	manifest := filepath.Join(dir, "producer.json")
	args = append(args, "--result-manifest="+manifest)
	if _, err = a.runBuild(ctx, r.Repository, "dda", args...); err != nil {
		return Result{}, err
	}
	return a.ExistingImage(ctx, r.Reference, manifest, r.Target)
}

// DeliverableImage tags the exact inspected local ID. The semver-shaped tag is
// content-specific, so rebuilding a mutable source tag changes pod templates.
func (a Adapter) DeliverableImage(ctx context.Context, r Result) (Result, error) {
	if err := r.Validate(r.Target); err != nil {
		return r, err
	}
	if r.Image == nil {
		return r, fmt.Errorf("image required")
	}
	i, err := a.InspectImage(ctx, r.Image.ID, r.Target)
	if err != nil {
		return r, err
	}
	repo := strings.Split(r.Image.Reference, "@")[0]
	if pos := strings.LastIndex(repo, ":"); pos > strings.LastIndex(repo, "/") {
		repo = repo[:pos]
	}
	ref := repo + ":7.99.0-e2ectl." + strings.TrimPrefix(i.ID, "sha256:")
	if _, err = a.run(ctx, "", "docker", "tag", i.ID, ref); err != nil {
		return r, err
	}
	delivered, err := a.InspectImage(ctx, ref, r.Target)
	if err != nil {
		return r, err
	}
	if delivered.ID != i.ID {
		return r, fmt.Errorf("delivered image identity mismatch")
	}
	copy := *r.Image
	r.Image = &copy
	r.Image.Delivered = ref
	return r, nil
}
func (a Adapter) VerifyImage(ctx context.Context, r Result) error {
	if err := r.Validate(r.Target); err != nil {
		return err
	}
	if r.Image == nil {
		return fmt.Errorf("image receipt required")
	}
	ref := r.Image.Delivered
	if ref == "" {
		return fmt.Errorf("installed deliverable tag missing")
	}
	i, err := a.InspectImage(ctx, ref, r.Target)
	if err != nil {
		return err
	}
	if i.ID != r.Image.ID {
		return fmt.Errorf("installed image tag changed")
	}
	return nil
}
func (a Adapter) ExistingPackage(ctx context.Context, path, manifest string, t Target) (Result, error) {
	if err := t.Validate(); err != nil {
		return Result{}, err
	}
	if filepath.Ext(path) != ".deb" || !filepath.IsAbs(path) {
		return Result{}, fmt.Errorf("exact absolute DEB path required")
	}
	file, err := DescribeFile(path)
	if err != nil {
		return Result{}, err
	}
	out, err := a.run(ctx, "", "dpkg-deb", "--show", "--showformat=${Package}\n${Version}\n${Architecture}\n", path)
	if err != nil {
		return Result{}, err
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(fields) != 3 || fields[0] != "datadog-agent" || fields[2] != t.Arch {
		return Result{}, fmt.Errorf("DEB metadata incompatible with target")
	}
	dependencies, err := a.run(ctx, "", "dpkg-deb", "-f", path, "Depends")
	if err != nil {
		return Result{}, err
	}
	contents, err := a.run(ctx, "", "dpkg-deb", "--contents", path)
	if err != nil {
		return Result{}, err
	}
	roles := packageRoles(string(contents))
	if len(roles) == 0 || roles[0] != "agent" {
		return Result{}, fmt.Errorf("DEB does not contain the core Agent executable")
	}
	r := Result{Schema: 1, Target: t, Provenance: Provenance{Producer: "existing-package"}, Package: &Package{File: file, Format: "deb", Name: fields[0], Version: fields[1], Roles: roles, Dependencies: strings.TrimSpace(string(dependencies))}}
	if manifest != "" {
		receipt, err := Read(manifest)
		if err != nil {
			return r, err
		}
		if receipt.Package == nil || !samePackage(receipt.Package, r.Package) {
			return r, fmt.Errorf("package differs from receipt")
		}
		r = receipt
	}
	return r, r.Validate(t)
}
func (a Adapter) ExistingBinary(manifest string, t Target, output string) (Result, error) {
	r, err := Read(manifest)
	if err != nil {
		return r, err
	}
	if r.Binary == nil {
		return r, fmt.Errorf("binary bundle receipt required")
	}
	if err = r.Validate(t); err != nil {
		return r, err
	}
	return Stage(r, output)
}
