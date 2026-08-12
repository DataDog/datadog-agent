// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kindinfra provisions a local kind Kubernetes cluster (and,
// optionally, a local fakeintake container) with no Pulumi involvement.
// It deliberately does nothing beyond infrastructure: agent installation is
// a separate concern, see cmd/envctl's install-agent subcommand.
package kindinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

const (
	defaultKubeVersion  = "1.31"
	kindReadinessWait   = "60s"
	fakeintakeImage     = "public.ecr.aws/datadog/fakeintake"
	fakeintakeRCKeySeed = "00112233445566778899aabbccddeeff0102030405060708090a0b0c0d0e0f"
	fakeintakeHostPort  = 30080
)

// Options configures the kindinfra provisioner.
type Options struct {
	KubeVersion       string
	WithoutFakeIntake bool
}

// Option mutates Options.
type Option func(*Options)

// WithKubeVersion selects the Kubernetes minor version (e.g. "1.31") to
// resolve a kind version/node-image pair for. Must be a version listed in
// components/kubernetes/kind_versions.json.
func WithKubeVersion(v string) Option { return func(o *Options) { o.KubeVersion = v } }

// WithoutFakeIntake skips starting the local fakeintake container.
func WithoutFakeIntake() Option { return func(o *Options) { o.WithoutFakeIntake = true } }

// Provisioner creates/destroys a local kind cluster (+ fakeintake). It
// implements provisioners.UntypedProvisioner.
type Provisioner struct {
	opts Options
}

var _ provisioners.UntypedProvisioner = &Provisioner{}

// New returns a new kindinfra Provisioner.
func New(opts ...Option) *Provisioner {
	o := Options{KubeVersion: defaultKubeVersion}
	for _, apply := range opts {
		apply(&o)
	}
	return &Provisioner{opts: o}
}

// ID returns the provisioner ID.
func (p *Provisioner) ID() string { return "kindinfra" }

// Provision creates the kind cluster and (optionally) the fakeintake
// container, returning their descriptions as RawResources. It's all-or-
// nothing: on any failure it rolls back whatever it already created, so a
// caller that doesn't persist partial state (e.g. because it only saves
// state after Provision returns successfully) can safely retry under the
// same stackName instead of hitting "cluster already exists".
func (p *Provisioner) Provision(ctx context.Context, stackName string, w io.Writer) (provisioners.RawResources, error) {
	resources := make(provisioners.RawResources)

	kubeconfig, err := p.createCluster(ctx, stackName, w)
	if err != nil {
		p.rollback(stackName, w)
		return nil, fmt.Errorf("creating kind cluster %q: %w", stackName, err)
	}

	clusterJSON, err := json.Marshal(compkube.ClusterOutput{
		ClusterName: stackName,
		KubeConfig:  kubeconfig,
	})
	if err != nil {
		p.rollback(stackName, w)
		return nil, err
	}
	resources["kubernetesCluster"] = clusterJSON

	if !p.opts.WithoutFakeIntake {
		fiOutput, err := p.startFakeintake(ctx, stackName, w)
		if err != nil {
			p.rollback(stackName, w)
			return nil, fmt.Errorf("starting fakeintake: %w", err)
		}
		fiJSON, err := json.Marshal(fiOutput)
		if err != nil {
			p.rollback(stackName, w)
			return nil, err
		}
		resources["fakeIntake"] = fiJSON
	}

	return resources, nil
}

// rollback tears down whatever Provision managed to create before it
// failed. It uses its own context (not the caller's, which may already be
// canceled/expired) since cleanup should still run on e.g. a timeout.
// kind delete cluster and docker rm -f are both no-ops (exit 0) against
// names that were never created, so calling this unconditionally on every
// failure path is safe even if nothing real exists yet.
func (p *Provisioner) rollback(stackName string, w io.Writer) {
	fmt.Fprintf(w, "provisioning %q failed, rolling back...\n", stackName)
	if err := p.Destroy(context.Background(), stackName, w); err != nil {
		fmt.Fprintf(w, "rollback of %q was incomplete, manual cleanup may be needed: %s\n", stackName, err)
	}
}

// Destroy deletes the kind cluster and stops the fakeintake container.
func (p *Provisioner) Destroy(ctx context.Context, stackName string, w io.Writer) error {
	var errs []string

	if !p.opts.WithoutFakeIntake {
		if err := runCmd(ctx, w, "docker", "rm", "-f", fakeintakeContainerName(stackName)); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if err := runCmd(ctx, w, "kind", "delete", "cluster", "--name", stackName); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("destroy errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (p *Provisioner) createCluster(ctx context.Context, name string, w io.Writer) (string, error) {
	versionConfig, err := compkube.GetKindVersionConfig(p.opts.KubeVersion)
	if err != nil {
		return "", err
	}
	// Pull straight from Docker Hub: the InternalDockerhubMirror() pull-through
	// cache used by the Pulumi path only exists on AWS CI runners, so a plain
	// local/dev machine gets nothing from it anyway.
	nodeImage := fmt.Sprintf("kindest/node:%s", versionConfig.NodeImageVersion)

	if err := runCmd(ctx, w, "kind", "create", "cluster",
		"--name", name, "--image", nodeImage, "--wait", kindReadinessWait); err != nil {
		return "", err
	}

	kubeconfig, err := exec.CommandContext(ctx, "kind", "get", "kubeconfig", "--name", name).Output()
	if err != nil {
		return "", fmt.Errorf("kind get kubeconfig: %w", err)
	}
	return string(kubeconfig), nil
}

func fakeintakeContainerName(stackName string) string {
	return "fakeintake-" + stackName
}

func (p *Provisioner) startFakeintake(ctx context.Context, stackName string, w io.Writer) (*fakeintake.FakeintakeOutput, error) {
	if err := runCmd(ctx, w, "docker", "run", "-d", "--rm",
		"--name", fakeintakeContainerName(stackName),
		"-p", fmt.Sprintf("%d:80", fakeintakeHostPort),
		fakeintakeImage, "--rc-key-data="+fakeintakeRCKeySeed); err != nil {
		return nil, err
	}

	host, err := localOutboundIP()
	if err != nil {
		return nil, err
	}

	return &fakeintake.FakeintakeOutput{
		Host:   host,
		Scheme: "http",
		Port:   fakeintakeHostPort,
		URL:    fmt.Sprintf("http://%s:%d", host, fakeintakeHostPort),
	}, nil
}

// localOutboundIP mirrors the trick used by the Pulumi-based local
// fakeintake (components/datadog/fakeintake/docker.go): dial out (no packet
// actually leaves the box) to learn which local IP would be used — that IP
// is reachable from inside the kind cluster's containers on the same docker
// network/host.
func localOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func runCmd(ctx context.Context, w io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	fmt.Fprintf(w, "+ %s %s\n", name, strings.Join(args, " "))
	return cmd.Run()
}
