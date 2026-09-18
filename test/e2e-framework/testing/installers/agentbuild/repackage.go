// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

func safeRepackageMount(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",:\n\r") || path == "/" || path == "/opt" || path == "/opt/datadog-agent" || strings.HasPrefix(path, "/opt/datadog-agent/") {
		return fmt.Errorf("repackage must never expose the host installed Agent tree: %q", path)
	}
	return nil
}

func ValidateRepackage(r RepackageRequest) error {
	for _, path := range []string{r.Repository, r.OutputDir} {
		if err := safeRepackageMount(path); err != nil {
			return err
		}
	}
	if err := r.Target.Native(); err != nil {
		return err
	}
	if err := ValidateReference(r.BuildImage); err != nil {
		return err
	}
	u, err := url.Parse(r.BasePackageURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !regexp.MustCompile(`^/[A-Za-z0-9/._-]+\.deb$`).MatchString(u.EscapedPath()) || !digestPattern.MatchString(r.BasePackageSHA256) {
		return fmt.Errorf("repackage requires credential-free HTTPS base-package-url and SHA256")
	}
	return nil
}
func (a Adapter) Repackage(ctx context.Context, r RepackageRequest) (Result, error) {
	if err := validateRequest(r.Request); err != nil {
		return Result{}, err
	}
	if err := ValidateRepackage(r); err != nil {
		return Result{}, err
	}
	image, err := a.InspectImage(ctx, r.BuildImage, r.Target)
	if err != nil {
		return Result{}, fmt.Errorf("isolated build image must already be available: %w", err)
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
	args := []string{"--rm", "-i", "--pull=never", "--workdir", r.Repository, "--mount", "type=bind,src=" + r.Repository + ",dst=" + r.Repository, "--mount", "type=bind,src=" + r.OutputDir + ",dst=" + r.OutputDir}
	// Linked worktrees store Git metadata outside the checkout. Read-only mounts
	// preserve honest source provenance without exposing the host installed Agent.
	for _, kind := range []string{"--git-common-dir", "--git-dir"} {
		out, err := a.run(ctx, r.Repository, "git", "rev-parse", "--path-format=absolute", kind)
		if err != nil {
			return Result{}, err
		}
		path := strings.TrimSpace(string(out))
		if err := safeRepackageMount(path); err != nil {
			return Result{}, err
		}
		if !strings.HasPrefix(path, r.Repository+"/") {
			args = append(args, "--mount", "type=bind,readonly,src="+path+",dst="+path)
		}
	}
	args = append(args, "--entrypoint", "dda", image.ID, "inv", "omnibus.build-repackaged-agent", "--base-package-url="+r.BasePackageURL, "--base-package-sha256="+r.BasePackageSHA256, "--result-manifest="+manifest)
	if _, err = a.runBuildContainer(ctx, r.Repository, args); err != nil {
		return Result{}, err
	}
	result, err := Read(manifest)
	if err != nil {
		return result, err
	}
	if result.Package == nil {
		return result, fmt.Errorf("Omnibus returned wrong artifact format")
	}
	if err = result.Validate(r.Target); err != nil {
		return result, err
	}
	verified, err := a.ExistingPackage(ctx, result.Package.File.Path, manifest, r.Target)
	if err != nil {
		return result, err
	}
	if verified.Provenance.Options == nil {
		verified.Provenance.Options = map[string]string{}
	}
	verified.Provenance.Options["buildImageReference"] = r.BuildImage
	verified.Provenance.Options["buildImageID"] = image.ID
	return Stage(verified, r.OutputDir)
}
