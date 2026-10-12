// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// ValidateAllowedRegistries validates host[:port] and host[:port]/repository-prefix
// entries. Wildcards, credentials, schemes, tags, and digests are not supported.
func ValidateAllowedRegistries(entries []string) error {
	for _, entry := range entries {
		if _, _, err := parseRegistryRule(entry); err != nil {
			return err
		}
	}
	return nil
}

func parseRegistryRule(entry string) (string, string, error) {
	u, err := url.Parse("oci://" + entry)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(entry, "*@%?#\\ \t\r\n") || strings.Contains(entry, "://") {
		return "", "", fmt.Errorf("invalid private_action_runner.allowed_registries entry %q: expected host[:port] or host[:port]/repository-prefix", entry)
	}
	registry, err := name.NewRegistry(u.Host, name.StrictValidation)
	if err != nil {
		return "", "", fmt.Errorf("invalid private_action_runner.allowed_registries entry %q: %w", entry, err)
	}
	path := strings.TrimPrefix(u.Path, "/")
	if path != "" {
		// Validate the path independently of registry-specific defaults: a Docker
		// Hub rule such as docker.io/team is a namespace prefix, not library/team.
		if _, err := name.NewRepository("validation.invalid/"+path, name.StrictValidation); err != nil || strings.Contains(path, ":") {
			return "", "", fmt.Errorf("invalid private_action_runner.allowed_registries repository prefix %q", entry)
		}
		for _, segment := range strings.Split(path, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return "", "", fmt.Errorf("invalid private_action_runner.allowed_registries repository prefix %q", entry)
			}
		}
	}
	return registry.RegistryStr(), path, nil
}

// AuthorizeRegistryURI checks the Agent policy before any network or cache
// access. An absent or empty allowlist denies every registry.
func AuthorizeRegistryURI(uri string, allowed []string) (name.Reference, error) {
	if err := ValidateAllowedRegistries(allowed); err != nil {
		return nil, err
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "oci" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.ContainsAny(uri, "%?#\\ \t\r\n") || !strings.HasPrefix(uri, "oci://") {
		return nil, fmt.Errorf("script URI must be an oci:// reference without credentials, query, or fragment")
	}
	ref, err := name.ParseReference(strings.TrimPrefix(uri, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("invalid script OCI reference: %w", err)
	}
	if _, _, err := parseRegistryRule(ref.Context().Name()); err != nil {
		return nil, fmt.Errorf("invalid script OCI repository: %w", err)
	}
	for _, entry := range allowed {
		host, path, _ := parseRegistryRule(entry) // Validated above.
		repository := ref.Context().RepositoryStr()
		if ref.Context().RegistryStr() == host && (path == "" || repository == path || strings.HasPrefix(repository, path+"/")) {
			return ref, nil
		}
	}
	return nil, fmt.Errorf("script OCI repository %q is not permitted by private_action_runner.allowed_registries", ref.Context().Name())
}
