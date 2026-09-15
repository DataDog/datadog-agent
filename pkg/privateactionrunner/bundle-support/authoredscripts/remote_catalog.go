// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"

	installercatalog "github.com/DataDog/datadog-agent/pkg/fleet/installer/catalog"
)

const authoredScriptPackagePrefix = "com.datadoghq.authoredscripts."

// RemoteCatalog adapts complete Fleet catalog snapshots to authored-script
// lookups. It is safe for Remote Config updates and action lookups to occur
// concurrently.
type RemoteCatalog struct {
	mu      sync.RWMutex
	catalog installercatalog.Catalog
}

// NewRemoteCatalog creates an empty authored-script catalog. Lookups fail
// closed until the first catalog snapshot is applied.
func NewRemoteCatalog() *RemoteCatalog {
	return &RemoteCatalog{}
}

// Replace atomically replaces the current catalog snapshot.
func (c *RemoteCatalog) Replace(next installercatalog.Catalog) error {
	if c == nil {
		return errors.New("authored-script remote catalog is not configured")
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("invalid package catalog: %w", err)
	}

	compatiblePackages := make(map[string]struct{})
	for _, pkg := range next.Packages {
		if !strings.HasPrefix(pkg.Name, authoredScriptPackagePrefix) {
			continue
		}
		if err := validateRemotePackage(pkg); err != nil {
			return fmt.Errorf("invalid authored-script package %q: %w", pkg.Name, err)
		}
		if !pkg.MatchesTarget(runtime.GOOS, runtime.GOARCH) {
			continue
		}
		if _, found := compatiblePackages[pkg.Name]; found {
			return fmt.Errorf("multiple compatible authored-script packages match %q", pkg.Name)
		}
		compatiblePackages[pkg.Name] = struct{}{}
	}

	packages := append([]installercatalog.Package(nil), next.Packages...)
	c.mu.Lock()
	c.catalog = installercatalog.Catalog{Packages: packages}
	c.mu.Unlock()
	return nil
}

// Lookup resolves the single compatible catalog package authorized for key.
func (c *RemoteCatalog) Lookup(key string) (Descriptor, error) {
	if c == nil {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, key)
	}
	if !strings.HasPrefix(key, authoredScriptPackagePrefix) || len(key) == len(authoredScriptPackagePrefix) {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, key)
	}

	packageName := strings.ToLower(key)
	c.mu.RLock()
	defer c.mu.RUnlock()

	var match *installercatalog.Package
	for i := range c.catalog.Packages {
		pkg := &c.catalog.Packages[i]
		if pkg.Name != packageName || !pkg.MatchesTarget(runtime.GOOS, runtime.GOARCH) {
			continue
		}
		if match != nil {
			return Descriptor{}, fmt.Errorf("%w: multiple compatible packages match %q", ErrPackageNotConfigured, key)
		}
		match = pkg
	}
	if match == nil {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, key)
	}

	return Descriptor{
		FQN:     key,
		Package: match.Name,
		Version: match.Version,
		URL:     match.URL,
		SHA256:  match.SHA256,
	}, nil
}

func validateRemotePackage(pkg installercatalog.Package) error {
	if pkg.Name != strings.ToLower(pkg.Name) {
		return errors.New("package name must be lowercase")
	}
	parsedURL, err := url.Parse(pkg.URL)
	if err != nil {
		return fmt.Errorf("could not parse OCI URL: %w", err)
	}
	if parsedURL.Scheme != "oci" {
		return fmt.Errorf("package URL uses unsupported scheme %q", parsedURL.Scheme)
	}
	if parsedURL.User != nil {
		return errors.New("OCI URL must not contain user information")
	}
	if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return errors.New("OCI URL must not contain a query or fragment")
	}

	reference, err := name.NewDigest(strings.TrimPrefix(pkg.URL, "oci://"), name.StrictValidation)
	if err != nil {
		return fmt.Errorf("package URL must contain a valid immutable OCI digest: %w", err)
	}
	expectedDigest := "sha256:" + pkg.SHA256
	if reference.DigestStr() != expectedDigest {
		return fmt.Errorf("OCI reference digest %q does not match expected digest %q", reference.DigestStr(), expectedDigest)
	}
	return nil
}
