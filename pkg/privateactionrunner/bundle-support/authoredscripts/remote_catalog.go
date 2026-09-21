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

	fleetcatalog "github.com/DataDog/datadog-agent/pkg/fleet/catalog"
	fleetcatalogrc "github.com/DataDog/datadog-agent/pkg/fleet/catalog/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
)

const authoredScriptPackagePrefix = "com.datadoghq.authoredscripts."

// NewRemoteCatalog creates an empty authored-script catalog and subscribes it
// to the requested Remote Config product. Lookups fail closed until the first
// complete catalog snapshot is applied.
func NewRemoteCatalog(client rcclient.Client, product string) (Catalog, error) {
	if client == nil {
		return nil, errors.New("Remote Config client is required for authored-script catalogs")
	}
	if product == "" {
		return nil, errors.New("Remote Config product is required for authored-script catalogs")
	}

	catalog := &remoteCatalog{}
	client.Subscribe(product, fleetcatalogrc.NewUpdateHandler(catalog.replace))
	return catalog, nil
}

type remoteCatalog struct {
	mu       sync.RWMutex
	packages map[string]fleetcatalog.Package
}

func (c *remoteCatalog) replace(next fleetcatalog.Catalog) error {
	if c == nil {
		return errors.New("authored-script remote catalog is not configured")
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("invalid package catalog: %w", err)
	}

	compatiblePackages := make(map[string]fleetcatalog.Package)
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
		compatiblePackages[pkg.Name] = pkg
	}

	c.mu.Lock()
	c.packages = compatiblePackages
	c.mu.Unlock()
	return nil
}

func (c *remoteCatalog) Lookup(fqn string) (Descriptor, error) {
	if c == nil {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, fqn)
	}
	if !strings.HasPrefix(fqn, authoredScriptPackagePrefix) || len(fqn) == len(authoredScriptPackagePrefix) {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, fqn)
	}

	packageName := strings.ToLower(fqn)
	c.mu.RLock()
	match, found := c.packages[packageName]
	c.mu.RUnlock()
	if !found {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrPackageNotConfigured, fqn)
	}

	return Descriptor{
		FQN:     fqn,
		Package: match.Name,
		Version: match.Version,
		URL:     match.URL,
		SHA256:  match.SHA256,
	}, nil
}

func validateRemotePackage(pkg fleetcatalog.Package) error {
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
