// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentbuild prepares verified Agent artifacts independently of CLI state
// and provisioning. Providers acquire; installers consume. There is no build cache.
package agentbuild

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func (t Target) Validate() error {
	if t.OS != "linux" || (t.Arch != "amd64" && t.Arch != "arm64") {
		return fmt.Errorf("unsupported artifact target %s/%s", t.OS, t.Arch)
	}
	return nil
}
func (t Target) Native() error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.OS != runtime.GOOS || t.Arch != runtime.GOARCH {
		return fmt.Errorf("native build required: target %s/%s differs from build host", t.OS, t.Arch)
	}
	return nil
}

type Provenance struct {
	Options       map[string]string `json:"options,omitempty"`
	Producer      string            `json:"producer"`
	Commit        string            `json:"commit,omitempty"`
	SourceSHA256  string            `json:"sourceSHA256,omitempty"`
	BaseReference string            `json:"baseReference,omitempty"`
	BaseIdentity  string            `json:"baseIdentity,omitempty"`
	Rebuilt       []string          `json:"rebuilt,omitempty"`
	Inherited     []string          `json:"inherited,omitempty"`
}
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode"`
	Link   string `json:"link,omitempty"`
}
type Tree struct {
	Root  string `json:"root"`
	Files []File `json:"files"`
}

// BinaryBundle preserves the producer's link-time prefix. It is not relocatable.
// The runtime is a complete embedded tree, not selected legacy dev/lib libraries.
type BinaryBundle struct {
	Executable     File   `json:"executable"`
	Runtime        Tree   `json:"runtime"`
	RuntimePrefix  string `json:"runtimePrefix"`
	Assets         Tree   `json:"assets"`
	RuntimeImageID string `json:"runtimeImageID,omitempty"`
	PythonPath     string `json:"pythonPath,omitempty"`
	PythonABI      string `json:"pythonABI,omitempty"`
	OSVersion      string `json:"osVersion,omitempty"`
}
type Image struct {
	Reference        string `json:"reference"`
	ID               string `json:"id"`
	RepositoryDigest string `json:"repositoryDigest,omitempty"`
	Delivered        string `json:"delivered,omitempty"`
}
type Package struct {
	Roles        []string `json:"roles,omitempty"`
	Dependencies string   `json:"dependencies,omitempty"`
	File         File     `json:"file"`
	Format       string   `json:"format"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
}

// Result is a versioned receipt of verified outputs, not a cache fingerprint.
// Exactly one format-specific payload is present. Profiles are trusted local
// build attestations bound to these bytes, not signatures or operator config.
type Result struct {
	Schema     int                        `json:"schema"`
	Target     Target                     `json:"target"`
	Provenance Provenance                 `json:"provenance"`
	Profile    *receivers.ProducerProfile `json:"profile,omitempty"`
	Binary     *BinaryBundle              `json:"binary,omitempty"`
	Image      *Image                     `json:"image,omitempty"`
	Package    *Package                   `json:"package,omitempty"`
}

type Invocation struct {
	Program string
	Args    []string
	Dir     string
	// StreamOutput keeps build logs and interactive safety prompts visible.
	// Structured inspection commands leave this false and return stdout.
	StreamOutput bool
}
type RunFunc func(context.Context, Invocation) ([]byte, error)

func Run(ctx context.Context, i Invocation) ([]byte, error) {
	cmd := exec.CommandContext(ctx, i.Program, i.Args...)
	cancelProcessTree(cmd)
	cmd.Dir = i.Dir
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	var out []byte
	var err error
	if i.StreamOutput {
		cmd.Stdout = os.Stdout
		err = cmd.Run()
	} else {
		out, err = cmd.Output()
	}
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", i.Program, err)
	}
	return out, nil
}

type Adapter struct{ Run RunFunc }

func (a Adapter) execute(ctx context.Context, i Invocation) ([]byte, error) {
	run := a.Run
	if run == nil {
		run = Run
	}
	return run(ctx, i)
}
func (a Adapter) run(ctx context.Context, dir, program string, args ...string) ([]byte, error) {
	return a.execute(ctx, Invocation{Program: program, Args: args, Dir: dir})
}
func (a Adapter) runBuild(ctx context.Context, dir, program string, args ...string) ([]byte, error) {
	return a.execute(ctx, Invocation{Program: program, Args: args, Dir: dir, StreamOutput: true})
}

type Request struct {
	Repository string
	OutputDir  string
	Target     Target
}
type BinaryRequest struct {
	Request
	Race bool
}
type ImageRequest struct {
	Request
	Reference         string
	BaseImage         string
	RebuildComponents []string
	Race              bool
}

// RepackageRequest executes the EXISTING omnibus task in an isolated Docker
// build image. No installed host /opt tree, Docker socket or privileged mount is
// exposed. The task's own safety prompt is never disabled.
type RepackageRequest struct {
	Request
	BuildImage        string
	BasePackageURL    string
	BasePackageSHA256 string
}
