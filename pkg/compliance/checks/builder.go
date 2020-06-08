// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrResourceNotSupported is returned when resource type is not supported by CheckBuilder
var ErrResourceNotSupported = errors.New("resource type not supported")

// ErrRuleScopeNotSupported is returned when resource scope is not supported
var ErrRuleScopeNotSupported = errors.New("rule scope not supported")

// Builder defines an interface to build checks from rules
type Builder interface {
	ChecksFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) ([]check.Check, error)
	Close() error
}

// BuilderOption defines a configuration option for the builder
type BuilderOption func(*builder) error

// WithInterval configures default check interval
func WithInterval(interval time.Duration) BuilderOption {
	return func(b *builder) error {
		b.checkInterval = interval
		return nil
	}
}

// WithHostname configures hostname used by checks
func WithHostname(hostname string) BuilderOption {
	return func(b *builder) error {
		b.hostname = hostname
		return nil
	}
}

// WithHostRootMount defines host root filesystem mount location
func WithHostRootMount(hostRootMount string) BuilderOption {
	return func(b *builder) error {
		log.Infof("Host root filesystem will be remapped to %s", hostRootMount)
		b.pathMapper = func(path string) string {
			return filepath.Join(hostRootMount, path)
		}
		return nil
	}
}

// WithDocker configures using docker
func WithDocker() BuilderOption {
	return func(b *builder) error {
		cli, err := newDockerClient()
		if err == nil {
			b.dockerClient = cli
		}
		return err
	}
}

// WithDockerClient configurs specific docker client
func WithDockerClient(cli DockerClient) BuilderOption {
	return func(b *builder) error {
		b.dockerClient = cli
		return nil
	}
}

// WithAudit configures using audit checks
func WithAudit() BuilderOption {
	return func(b *builder) error {
		cli, err := newAuditClient()
		if err == nil {
			b.auditClient = cli
		}
		return err
	}
}

// WithAuditClient configures using specific audit client
func WithAuditClient(cli AuditClient) BuilderOption {
	return func(b *builder) error {
		b.auditClient = cli
		return nil
	}
}

// MayFail configures a builder option to succeed on failures and logs an error
func MayFail(o BuilderOption) BuilderOption {
	return func(b *builder) error {
		if err := o(b); err != nil {
			log.Warnf("Ignoring builder initialization failure: %v", err)
		}
		return nil
	}
}

// NewBuilder constructs a check builder
func NewBuilder(reporter compliance.Reporter, options ...BuilderOption) (Builder, error) {
	b := &builder{
		reporter:      reporter,
		checkInterval: 20 * time.Minute,
	}

	for _, o := range options {
		if err := o(b); err != nil {
			return nil, err
		}

	}
	return b, nil
}

type builder struct {
	checkInterval time.Duration

	reporter     compliance.Reporter
	dockerClient DockerClient
	auditClient  AuditClient
	hostname     string
	pathMapper   pathMapper
}

const (
	checkKindFile    = checkKind("file")
	checkKindProcess = checkKind("process")
	checkKindCommand = checkKind("command")
	checkKindDocker  = checkKind("docker")
	checkKindAudit   = checkKind("audit")
)

func (b *builder) Close() error {
	if b.dockerClient != nil {
		if err := b.dockerClient.Close(); err != nil {
			return err
		}
	}
	if b.auditClient != nil {
		if err := b.auditClient.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (b *builder) ChecksFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) ([]check.Check, error) {
	ruleScope, err := b.getRuleScope(meta, rule)
	if err != nil {
		return nil, err
	}

	var checks []check.Check
	for _, resource := range rule.Resources {
		// TODO: there will be some logic introduced here to allow for composite checks,
		// to support overrides of reported values, e.g.:
		// default value checked in a file but can be overwritten by a process
		// argument. Currently we treat them as independent checks.

		if check, err := b.checkFromRule(meta, rule.ID, ruleScope, resource); err == nil {
			checks = append(checks, check)
		}
	}
	return checks, nil
}

func (b *builder) getRuleScope(meta *compliance.SuiteMeta, rule *compliance.Rule) (string, error) {
	if rule.Scope.Docker {
		return "docker", nil
	}

	if len(rule.Scope.Kubernetes) > 0 {
		// TODO: resource actual scope for Kubernetes role here
		return "worker", nil
	}
	return "", ErrRuleScopeNotSupported
}

func (b *builder) checkFromRule(meta *compliance.SuiteMeta, ruleID string, ruleScope string, resource compliance.Resource) (check.Check, error) {
	switch {
	case resource.File != nil:
		return newFileCheck(b.baseCheck(ruleID, checkKindFile, ruleScope, meta), b.pathMapper, resource.File)
	case resource.Docker != nil:
		if b.dockerClient == nil {
			return nil, log.Errorf("%s: skipped - docker client not initialized", ruleID)
		}
		return newDockerCheck(b.baseCheck(ruleID, checkKindFile, ruleScope, meta), b.dockerClient, resource.Docker)
	case resource.Process != nil:
		return newProcessCheck(b.baseCheck(ruleID, checkKindProcess, ruleScope, meta), resource.Process)
	case resource.Command != nil:
		return newCommandCheck(b.baseCheck(ruleID, checkKindCommand, ruleScope, meta), resource.Command)
	case resource.Audit != nil:
		if b.auditClient == nil {
			return nil, log.Errorf("%s: skipped - audit client not initialized", ruleID)
		}
		return newAuditCheck(b.baseCheck(ruleID, checkKindAudit, ruleScope, meta), b.auditClient, resource.Audit)
	default:
		log.Errorf("%s: resource not supported", ruleID)
		return nil, ErrResourceNotSupported
	}
}

func (b *builder) baseCheck(ruleID string, kind checkKind, ruleScope string, meta *compliance.SuiteMeta) baseCheck {
	return baseCheck{
		id:        newCheckID(ruleID, kind),
		kind:      kind,
		interval:  b.checkInterval,
		reporter:  b.reporter,
		framework: meta.Framework,
		version:   meta.Version,

		ruleID:       ruleID,
		resourceType: ruleScope,
		resourceID:   b.hostname,
	}
}

func newCheckID(ruleID string, kind checkKind) check.ID {
	return check.ID(fmt.Sprintf("%s:%s", ruleID, kind))
}
