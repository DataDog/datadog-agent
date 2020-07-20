// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	cache "github.com/patrickmn/go-cache"
)

// ErrResourceNotSupported is returned when resource type is not supported by CheckBuilder
var ErrResourceNotSupported = errors.New("resource type not supported")

// ErrRuleScopeNotSupported is returned when resource scope is not supported
var ErrRuleScopeNotSupported = errors.New("rule scope not supported")

// Builder defines an interface to build checks from rules
type Builder interface {
	ChecksFromFile(file string, onCheck compliance.CheckVisitor) error
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
func WithDockerClient(cli env.DockerClient) BuilderOption {
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
func WithAuditClient(cli env.AuditClient) BuilderOption {
	return func(b *builder) error {
		b.auditClient = cli
		return nil
	}
}

// WithKubernetesClient allows specific Kubernetes client
func WithKubernetesClient(cli env.KubeClient) BuilderOption {
	return func(b *builder) error {
		b.kubeClient = cli
		return nil
	}
}

// SuiteMatcher checks if a compliance suite is included
type SuiteMatcher func(*compliance.SuiteMeta) bool

// WithMatchSuite configures builder to use a suite matcher
func WithMatchSuite(matcher SuiteMatcher) BuilderOption {
	return func(b *builder) error {
		b.suiteMatcher = matcher
		return nil
	}
}

// RuleMatcher checks if a compliance rule is included
type RuleMatcher func(*compliance.Rule) bool

// WithMatchRule configures builder to use a suite matcher
func WithMatchRule(matcher RuleMatcher) BuilderOption {
	return func(b *builder) error {
		b.ruleMatcher = matcher
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

// IsFramework matches a compliance suite by the name of the framework
func IsFramework(framework string) SuiteMatcher {
	return func(s *compliance.SuiteMeta) bool {
		return s.Framework == framework
	}
}

// IsRuleID matches a compliance rule by ID
func IsRuleID(ruleID string) RuleMatcher {
	return func(r *compliance.Rule) bool {
		return r.ID == ruleID
	}
}

// NewBuilder constructs a check builder
func NewBuilder(reporter event.Reporter, options ...BuilderOption) (Builder, error) {
	b := &builder{
		reporter:      reporter,
		checkInterval: 20 * time.Minute,
		etcGroupPath:  "/etc/group",
	}

	for _, o := range options {
		if err := o(b); err != nil {
			return nil, err
		}

	}

	b.valueCache = cache.New(
		b.checkInterval/2,
		b.checkInterval/4,
	)
	return b, nil
}

type builder struct {
	checkInterval time.Duration

	reporter   event.Reporter
	valueCache *cache.Cache

	hostname     string
	pathMapper   pathMapper
	etcGroupPath string

	suiteMatcher SuiteMatcher
	ruleMatcher  RuleMatcher

	dockerClient env.DockerClient
	auditClient  env.AuditClient
	kubeClient   env.KubeClient
}

const (
	checkKindFile          = checkKind("file")
	checkKindProcess       = checkKind("process")
	checkKindCommand       = checkKind("command")
	checkKindDocker        = checkKind("docker")
	checkKindAudit         = checkKind("audit")
	checkKindGroup         = checkKind("group")
	checkKindKubeApiserver = checkKind("kubeapiserver")
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

func (b *builder) ChecksFromFile(file string, onCheck compliance.CheckVisitor) error {
	suite, err := compliance.ParseSuite(file)
	if err != nil {
		return err
	}

	if b.suiteMatcher != nil && !b.suiteMatcher(&suite.Meta) {
		log.Tracef("%s/%s: skipped suite from %s", suite.Meta.Name, suite.Meta.Version, file)
		return nil
	}

	log.Infof("%s/%s: loading suite from %s", suite.Meta.Name, suite.Meta.Version, file)
	for _, r := range suite.Rules {
		if b.ruleMatcher != nil && !b.ruleMatcher(&r) {
			log.Tracef("%s/%s: skipped rule %s from %s", suite.Meta.Name, suite.Meta.Version, r.ID, file)
			continue
		}

		log.Debugf("%s/%s: loading rule %s", suite.Meta.Name, suite.Meta.Version, r.ID)
		checks, err := b.ChecksFromRule(&suite.Meta, &r)
		if err != nil {
			return err
		}
		for _, check := range checks {
			log.Debugf("%s/%s: init check %s", suite.Meta.Name, suite.Meta.Version, check.ID())
			err = onCheck(check)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *builder) ChecksFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) ([]check.Check, error) {
	ruleScope, err := b.getRuleScope(meta, rule)
	if err != nil {
		return nil, err
	}

	eligible, err := b.hostMatcher(ruleScope, rule)
	if err != nil {
		return nil, err
	}
	if !eligible {
		log.Debugf("rule %s/%s discarded by hostMatcher", meta.Framework, rule.ID)
		return nil, nil
	}

	var checks []check.Check
	for _, resource := range rule.Resources {
		// TODO: there will be some logic introduced here to allow for composite checks,
		// to support overrides of reported values, e.g.:
		// default value checked in a file but can be overwritten by a process
		// argument. Currently we treat them as independent checks.

		if check, err := b.checkFromRule(meta, rule.ID, ruleScope, resource); err == nil {
			checks = append(checks, check)
		} else {
			return nil, fmt.Errorf("unable to create check for rule: %s/%s, err: %v", meta.Framework, rule.ID, err)
		}
	}
	return checks, nil
}

func (b *builder) getRuleScope(meta *compliance.SuiteMeta, rule *compliance.Rule) (string, error) {
	switch {
	case rule.Scope.Docker:
		return compliance.DockerScope, nil
	case rule.Scope.KubernetesNode:
		return compliance.KubernetesNodeScope, nil
	case rule.Scope.KubernetesCluster:
		return compliance.KubernetesClusterScope, nil
	default:
		return "", ErrRuleScopeNotSupported
	}
}

func (b *builder) hostMatcher(scope string, rule *compliance.Rule) (bool, error) {
	if scope == compliance.KubernetesNodeScope {
		if config.IsKubernetes() {
			labels, err := hostinfo.GetNodeLabels()
			if err != nil {
				return false, err
			}

			return b.isKubernetesNodeEligible(rule.HostSelector, labels), nil
		}

		log.Infof("rule %s discarded as we're not running on a Kube node", rule.ID)
		return false, nil
	}

	return true, nil
}

func (b *builder) isKubernetesNodeEligible(hostSelector *compliance.HostSelector, nodeLabels map[string]string) bool {
	if hostSelector == nil {
		return true
	}

	// No filtering, no need to fetch node labels
	if len(hostSelector.KubernetesNodeLabels) == 0 && len(hostSelector.KubernetesNodeRole) == 0 {
		return true
	}

	// Check selector
	for _, selector := range hostSelector.KubernetesNodeLabels {
		value, found := nodeLabels[selector.Label]
		if !found {
			return false
		}

		if value != selector.Value {
			return false
		}
	}

	if len(hostSelector.KubernetesNodeRole) > 0 {
		// Specific node role matching as multiple syntax exists
		for key, value := range nodeLabels {
			key, value = hostinfo.LabelPreprocessor(key, value)
			if key == hostinfo.NormalizedRoleLabel && value == hostSelector.KubernetesNodeRole {
				return true
			}
		}

		return false
	}

	return true
}

func (b *builder) checkFromRule(meta *compliance.SuiteMeta, ruleID string, ruleScope string, resource compliance.Resource) (check.Check, error) {
	switch {
	case resource.File != nil:
		return newFileCheck(b.baseCheck(ruleID, checkKindFile, ruleScope, meta), resource.File)
	case resource.Group != nil:
		return newGroupCheck(b.baseCheck(ruleID, checkKindGroup, ruleScope, meta), resource.Group)
	case resource.Process != nil:
		return newProcessCheck(b.baseCheck(ruleID, checkKindProcess, ruleScope, meta), resource.Process)
	case resource.Command != nil:
		return newCommandCheck(b.baseCheck(ruleID, checkKindCommand, ruleScope, meta), resource.Command)
	case resource.Audit != nil:
		if b.auditClient == nil {
			return nil, log.Errorf("%s: skipped - audit client not initialized", ruleID)
		}
		return newAuditCheck(b.baseCheck(ruleID, checkKindAudit, ruleScope, meta), resource.Audit)
	case resource.Docker != nil:
		if b.dockerClient == nil {
			return nil, log.Errorf("%s: skipped - docker client not initialized", ruleID)
		}
		return newDockerCheck(b.baseCheck(ruleID, checkKindFile, ruleScope, meta), resource.Docker)
	case resource.KubeApiserver != nil:
		if b.kubeClient == nil {
			return nil, log.Errorf("%s: skipped - kube client not initialized", ruleID)
		}
		return newKubeapiserverCheck(b.baseCheck(ruleID, checkKindKubeApiserver, ruleScope, meta), resource.KubeApiserver)
	default:
		log.Errorf("%s: resource not supported", ruleID)
		return nil, ErrResourceNotSupported
	}
}

func (b *builder) baseCheck(ruleID string, kind checkKind, ruleScope string, meta *compliance.SuiteMeta) baseCheck {
	return baseCheck{
		Env: b,

		name:     ruleID,
		id:       newCheckID(ruleID, kind),
		kind:     kind,
		interval: b.checkInterval,

		framework: meta.Framework,
		suiteName: meta.Name,
		version:   meta.Version,

		ruleID:       ruleID,
		resourceType: ruleScope,
		resourceID:   b.hostname,
	}
}

func newCheckID(ruleID string, kind checkKind) check.ID {
	return check.ID(fmt.Sprintf("%s:%s", ruleID, kind))
}

func (b *builder) Reporter() event.Reporter {
	return b.reporter
}

func (b *builder) DockerClient() env.DockerClient {
	return b.dockerClient
}

func (b *builder) AuditClient() env.AuditClient {
	return b.auditClient
}

func (b *builder) KubeClient() env.KubeClient {
	return b.kubeClient
}

func (b *builder) Hostname() string {
	return b.hostname
}

func (b *builder) EtcGroupPath() string {
	return b.etcGroupPath
}

func (b *builder) NormalizePath(path string) string {
	if b.pathMapper == nil {
		return path
	}
	return b.pathMapper(path)
}

func (b *builder) ResolveValueFrom(valueFrom compliance.ValueFrom) (string, error) {
	for _, source := range valueFrom {
		key := source.String()
		if value, exists := b.valueCache.Get(key); exists {
			return value.(string), nil
		}

		value, err := b.getValueFromSource(source)
		if err != nil {
			log.Debugf("Failed to fetch %s: %v", key, err)
			continue
		}

		b.valueCache.Set(key, value, cache.DefaultExpiration)
		return value, nil

	}
	return "", errors.New("failed to resolve")
}

func (b *builder) getValueFromSource(source compliance.ValueSource) (string, error) {
	switch {
	case source.Command != nil:
		return b.getValueFromCommand(source.Command)
	case source.Process != nil:
		return b.getValueFromProcess(source.Process)
	case source.File != nil:
		return b.getValueFromFile(source.File)
	}
	return "", errors.New("unsupported value source")
}

func (b *builder) getValueFromCommand(cmd *compliance.ValueFromCommand) (string, error) {
	log.Debugf("Resolving value from command: %v", cmd)

	context, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var execCommand compliance.BinaryCmd
	if cmd.BinaryCmd != nil {
		execCommand = *cmd.BinaryCmd
	} else if cmd.ShellCmd != nil {
		execCommand = shellCmdToBinaryCmd(cmd.ShellCmd)
	} else {
		return "", errors.New("invalid command source value")
	}

	exitCode, stdout, err := commandRunner(context, execCommand.Name, execCommand.Args, true)
	if exitCode == -1 && err != nil {
		return "", fmt.Errorf("command '%v' execution failed, error: %v", cmd, err)
	}
	return string(stdout), nil
}

func (b *builder) getValueFromProcess(p *compliance.ValueFromProcess) (string, error) {
	log.Debugf("Resolving value from process: %v", p)

	processes, err := getProcesses(cacheValidity)
	if err != nil {
		return "", log.Errorf("Unable to fetch processes: %v", err)
	}

	matchedProcesses := processes.findProcessesByName(p.Name)
	for _, mp := range matchedProcesses {
		flagValues := parseProcessCmdLine(mp.Cmdline)
		if flagValue, found := flagValues[p.Flag]; found {
			return flagValue, nil
		}
	}
	return "", fmt.Errorf("failed to get: %v", p)
}

func (b *builder) getValueFromFile(f *compliance.ValueFromFile) (string, error) {
	switch f.Kind {
	case compliance.PropertyKindJSONQuery:
		return queryValueFromFile(f.Path, f.Property, jsonGetter)
	case compliance.PropertyKindYAMLQuery:
		return queryValueFromFile(f.Path, f.Property, yamlGetter)
	default:
		return "", ErrPropertyKindNotSupported
	}
}
