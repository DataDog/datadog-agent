// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package buildprovider owns explicit typed CLI registrations. Public adapters
// have no dependency on this registry, CLI config, snapshots or Pulumi.
package buildprovider

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	bc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

// Result wrappers make incompatible provider registration a type error.
type BinaryResult struct{ agentbuild.Result }
type ImageResult struct{ agentbuild.Result }
type PackageResult struct{ agentbuild.Result }
type Request struct {
	Target    agentbuild.Target
	OutputDir string
	Adapter   agentbuild.Adapter
}
type Definition[R any] struct {
	ID, Description string
	validate        func(*config.BuildSelection) error
	routing         func(*config.BuildSelection) error
	prepare         func(context.Context, *config.BuildSelection, Request) (R, error)
}

func Define[P, R any](id, description string, schema *configschema.Schema[P], validate func(P) error, prepare func(context.Context, P, Request) (R, error), routing ...func(P) error) Definition[R] {
	if id == "" || description == "" || schema == nil || prepare == nil {
		panic("provider requires ID, description, schema, preparation")
	}
	decode := func(s *config.BuildSelection) (P, error) {
		var p P
		var err error
		if s.SectionNode != nil {
			p, _, err = schema.DecodeNode(s.SectionNode, "agent.build."+id)
		} else {
			p, _, err = schema.Decode(s.Section, "agent.build."+id)
		}
		if err == nil && validate != nil {
			err = validate(p)
		}
		return p, err
	}
	return Definition[R]{ID: id, Description: description, routing: func(s *config.BuildSelection) error {
		p, err := decode(s)
		if err != nil {
			return err
		}
		if len(routing) != 1 {
			return fmt.Errorf("provider %s has no verified receiver capability contract", id)
		}
		return routing[0](p)
	}, validate: func(s *config.BuildSelection) error { _, err := decode(s); return err }, prepare: func(ctx context.Context, s *config.BuildSelection, r Request) (R, error) {
		p, err := decode(s)
		if err != nil {
			var zero R
			return zero, err
		}
		return prepare(ctx, p, r)
	}}
}

type Registry[R any] struct{ definitions map[string]Definition[R] }

func NewRegistry[R any](definitions ...Definition[R]) Registry[R] {
	r := Registry[R]{definitions: map[string]Definition[R]{}}
	for _, d := range definitions {
		if _, ok := r.definitions[d.ID]; ok {
			panic("duplicate provider " + d.ID)
		}
		r.definitions[d.ID] = d
	}
	return r
}
func (r Registry[R]) Validate(s *config.BuildSelection) error {
	if s == nil {
		return fmt.Errorf("agent.build selection required")
	}
	d, ok := r.definitions[s.Provider]
	if !ok {
		return fmt.Errorf("provider %q is not compatible with this installer", s.Provider)
	}
	return d.validate(s)
}
func (r Registry[R]) ValidateRouting(s *config.BuildSelection) error {
	if err := r.Validate(s); err != nil {
		return err
	}
	return r.definitions[s.Provider].routing(s)
}
func receiptProfile(path string) error {
	if path == "" {
		return fmt.Errorf("explicit receiver requires a previously generated compatible provider manifest")
	}
	r, err := agentbuild.Read(path)
	if err != nil {
		return err
	}
	if err = r.Validate(r.Target); err != nil {
		return err
	}
	if r.Profile == nil {
		return fmt.Errorf("artifact receipt has no receiver capability evidence")
	}
	return nil
}

func (r Registry[R]) Prepare(ctx context.Context, s *config.BuildSelection, request Request) (R, error) {
	var zero R
	if err := r.Validate(s); err != nil {
		return zero, err
	}
	return r.definitions[s.Provider].prepare(ctx, s, request)
}
func absolute(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("artifact/repository paths must be absolute (not relative to stored config)")
	}
	return nil
}
func optionalManifest(path string) error {
	if path == "" {
		return nil
	}
	return absolute(path)
}
func buildRequest(repo string, r Request) agentbuild.Request {
	return agentbuild.Request{Repository: repo, OutputDir: r.OutputDir, Target: r.Target}
}

// Explicit registration only. The installer selects its typed registry; main
// command and environment drivers never dispatch on artifact format/task names.
var Images = NewRegistry(
	Define("existing-image", "Consume a local image without building", bc.ExistingImageSchema, func(p bc.ExistingImage) error {
		if err := agentbuild.ValidateReference(p.Reference); err != nil {
			return err
		}
		return optionalManifest(p.Manifest)
	}, func(ctx context.Context, p bc.ExistingImage, r Request) (ImageResult, error) {
		out, err := r.Adapter.ExistingImage(ctx, p.Reference, p.Manifest, r.Target)
		return ImageResult{out}, err
	}, func(p bc.ExistingImage) error { return receiptProfile(p.Manifest) }),
	Define("invoke-image", "Prepare a hacky development image", bc.InvokeImageSchema, func(p bc.InvokeImage) error {
		if err := absolute(p.Repository); err != nil {
			return err
		}
		if err := agentbuild.ValidateReference(p.Reference); err != nil {
			return err
		}
		return agentbuild.ValidateReference(p.BaseImage)
	}, func(ctx context.Context, p bc.InvokeImage, r Request) (ImageResult, error) {
		out, err := r.Adapter.BuildImage(ctx, agentbuild.ImageRequest{Request: buildRequest(p.Repository, r), Reference: p.Reference, BaseImage: p.BaseImage, RebuildComponents: p.RebuildComponents, Race: p.Race})
		return ImageResult{out}, err
	}, func(p bc.InvokeImage) error {
		if p.BaseImage != "registry.datadoghq.com/agent:7.83.0" {
			return fmt.Errorf("explicit receiver requires the tested 7.83.0 image base")
		}
		return nil
	}),
)
var Binaries = NewRegistry(
	Define("existing-binary", "Consume a verified binary/runtime bundle", bc.ExistingBinarySchema, func(p bc.ExistingBinary) error { return absolute(p.Manifest) }, func(_ context.Context, p bc.ExistingBinary, r Request) (BinaryResult, error) {
		out, err := r.Adapter.ExistingBinary(p.Manifest, r.Target, r.OutputDir)
		return BinaryResult{out}, err
	}, func(p bc.ExistingBinary) error {
		r, err := agentbuild.Read(p.Manifest)
		if err != nil {
			return err
		}
		return r.RequireBinaryRouting()
	}),
	Define("invoke-binary", "Build the core Agent and full embedded runtime", bc.InvokeBinarySchema, func(p bc.InvokeBinary) error { return absolute(p.Repository) }, func(ctx context.Context, p bc.InvokeBinary, r Request) (BinaryResult, error) {
		out, err := r.Adapter.BuildBinary(ctx, agentbuild.BinaryRequest{Request: buildRequest(p.Repository, r), Race: p.Race})
		return BinaryResult{out}, err
	}, func(bc.InvokeBinary) error { return nil }),
)
var Packages = NewRegistry(
	Define("existing-package", "Consume an exact existing DEB without building", bc.ExistingPackageSchema, func(p bc.ExistingPackage) error {
		if err := absolute(p.Path); err != nil {
			return err
		}
		if filepath.Ext(p.Path) != ".deb" {
			return fmt.Errorf("only DEB packages supported")
		}
		return optionalManifest(p.Manifest)
	}, func(ctx context.Context, p bc.ExistingPackage, r Request) (PackageResult, error) {
		out, err := r.Adapter.ExistingPackage(ctx, p.Path, p.Manifest, r.Target)
		if err == nil {
			out, err = agentbuild.Stage(out, r.OutputDir)
		}
		return PackageResult{out}, err
	}, func(p bc.ExistingPackage) error { return receiptProfile(p.Manifest) }),
	Define("omnibus-repackage", "Run existing Omnibus repack in an isolated native build container", bc.OmnibusRepackageSchema, func(p bc.OmnibusRepackage) error {
		if err := absolute(p.Repository); err != nil {
			return err
		}
		return agentbuild.ValidateReference(p.BuildImage)
	}, func(ctx context.Context, p bc.OmnibusRepackage, r Request) (PackageResult, error) {
		out, err := r.Adapter.Repackage(ctx, agentbuild.RepackageRequest{Request: buildRequest(p.Repository, r), BuildImage: p.BuildImage, BasePackageURL: p.BasePackageURL, BasePackageSHA256: p.BasePackageSHA256})
		return PackageResult{out}, err
	}),
)
