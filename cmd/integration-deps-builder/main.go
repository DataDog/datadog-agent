// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command integration-deps-builder generates the on-demand python dependency
// manifest and builds the fat dependency OCI package from a python
// site-packages tree.
//
// It is a local development tool for the on-demand dependencies POC: point it
// at the site-packages directory of a fully built agent, and it produces:
//   - the dependency manifest (check name -> required python import names)
//     for on_demand_python_deps.manifest_path
//   - an OCI layout directory that the fleet installer daemon can install
//     through a catalog entry with a file:// URL
//   - optionally a push to an OCI registry
//
// Usage:
//
//	go run ./cmd/integration-deps-builder \
//	  --site-packages <agent>/embedded/lib/python3.12/site-packages \
//	  --version <agent version> \
//	  --manifest-out python_deps_manifest.json \
//	  --out deps-package
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	installeroci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultPackageName = "datadog-agent-integration-deps"

func main() {
	sitePackagesPath := flag.String("site-packages", "", "path of a python site-packages tree (required)")
	packageName := flag.String("name", defaultPackageName, "name of the dependency package")
	packageVersion := flag.String("version", version.AgentPackageVersion, "version of the dependency package (1:1 with the agent version)")
	manifestOut := flag.String("manifest-out", "", "path to write the dependency manifest JSON")
	outDir := flag.String("out", "", "path to write the OCI layout directory")
	pushRef := flag.String("push", "", "optional OCI reference to push the package index to (e.g. localhost:5000/datadog-agent-integration-deps)")
	flag.Parse()

	if *sitePackagesPath == "" {
		fmt.Fprintln(os.Stderr, "--site-packages is required")
		flag.Usage()
		os.Exit(2)
	}
	if *manifestOut == "" && *outDir == "" && *pushRef == "" {
		fmt.Fprintln(os.Stderr, "nothing to do: pass at least one of --manifest-out, --out, --push")
		os.Exit(2)
	}

	site, err := installeroci.ParseSitePackages(*sitePackagesPath)
	if err != nil {
		fatal(err)
	}

	manifest := site.DepsManifest()
	if *manifestOut != "" {
		data, err := json.MarshalIndent(map[string]any{"checks": manifest}, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*manifestOut, append(data, '\n'), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote dependency manifest for %d checks to %s\n", len(manifest), *manifestOut)
	}

	dists := site.DepsPackageDists()
	if len(dists) == 0 {
		fmt.Println("no third-party dependencies found, nothing to package")
		return
	}
	fmt.Printf("packaging %d third-party dependencies:\n", len(dists))
	for _, dist := range dists {
		fmt.Printf("  %s\n", filepath.Base(dist.Dir))
	}

	image, err := site.BuildDepsPackage(*packageName, *packageVersion, dists)
	if err != nil {
		fatal(err)
	}

	index := mutate.IndexMediaType(mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add: image,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           runtime.GOOS,
					Architecture: runtime.GOARCH,
				},
			},
		},
	), types.OCIImageIndex)

	if *outDir != "" {
		if _, err := layout.Write(*outDir, index); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote OCI layout to %s\n", *outDir)
	}
	if *pushRef != "" {
		ref, err := name.ParseReference(*pushRef)
		if err != nil {
			fatal(err)
		}
		if err := remote.WriteIndex(ref, index); err != nil {
			fatal(err)
		}
		fmt.Printf("pushed package index to %s\n", *pushRef)
	}

	digest, err := index.Digest()
	if err != nil {
		fatal(err)
	}
	fmt.Printf(`
package %s version %s digest %s

To test locally, register the package in the installer daemon catalog:

  curl -X POST --unix-socket /opt/datadog-packages/run/installer.sock \
    -H 'Content-Type: application/json' http://installer/catalog -d '{
      "packages": [{
        "package": %q,
        "version": %q,
        "sha256": "",
        "url": "file://%s",
        "size": 0
      }]
    }'

Then point on_demand_python_deps.manifest_path at the manifest and enable
on_demand_python_deps in datadog.yaml.
`, *packageName, *packageVersion, digest.String(), *packageName, *packageVersion, absPath(*outDir))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "integration-deps-builder: %v\n", err)
	os.Exit(1)
}

func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
