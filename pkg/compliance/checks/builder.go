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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	cache "github.com/patrickmn/go-cache"
)

// ErrResourceNotSupported is returned when resource type is not supported by Builder
var ErrResourceNotSupported = errors.New("resource type not supported")

// ErrRuleScopeNotSupported is returned when resource scope is not supported
var ErrRuleScopeNotSupported = errors.New("rule scope not supported")

// ErrRuleDoesNotApply is returned when a rule cannot be applied to the current environment
var ErrRuleDoesNotApply = errors.New("rule does not apply to this environment")

const (
	builderFuncExec        = "exec"
	builderFuncShell       = "shell"
	builderFuncProcessFlag = "process.flag"
	builderFuncJSON        = "json"
	builderFuncYAML        = "yaml"
)

// Builder defines an interface to build checks from rules
type Builder interface {
	ChecksFromFile(file string, onCheck compliance.CheckVisitor) error
	CheckFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) (check.Check, error)
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

type pathMapper func(string) string

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
		check, err := b.CheckFromRule(&suite.Meta, &r)

		if err != nil {
			if err == ErrRuleDoesNotApply {
				continue
			}
			return err
		}

		log.Debugf("%s/%s: init check %s", suite.Meta.Name, suite.Meta.Version, check.ID())
		err = onCheck(check)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *builder) CheckFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) (check.Check, error) {
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
		return nil, ErrRuleDoesNotApply
	}

	return b.newCheck(meta, ruleScope, rule), nil
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

		log.Infof("rule %s discarded as we're not running on a Kubernetes node", rule.ID)
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

func (b *builder) newCheck(meta *compliance.SuiteMeta, ruleScope string, rule *compliance.Rule) *complianceCheck {
	checkable, err := newResourceCheckList(b, rule.ID, rule.Resources)

	if err != nil {
		log.Warnf("%s: check failed to initialize: %v", rule.ID, err)
	}

	// We capture err as configuration error but do not prevent check creation
	return &complianceCheck{
		Env: b,

		name:     rule.ID,
		ruleID:   rule.ID,
		interval: b.checkInterval,

		framework: meta.Framework,
		suiteName: meta.Name,
		version:   meta.Version,

		resourceType: ruleScope,
		resourceID:   b.hostname,
		configError:  err,
		checkable:    checkable,
	}
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

func (b *builder) EvaluateFromCache(ev eval.Evaluatable) (interface{}, error) {

	instance := &eval.Instance{
		Functions: eval.FunctionMap{
			builderFuncShell:       b.withValueCache(builderFuncShell, evalCommandShell),
			builderFuncExec:        b.withValueCache(builderFuncExec, evalCommandExec),
			builderFuncProcessFlag: b.withValueCache(builderFuncProcessFlag, evalProcessFlag),
			builderFuncJSON:        b.withValueCache(builderFuncJSON, b.evalValueFromFile(jsonGetter)),
			builderFuncYAML:        b.withValueCache(builderFuncYAML, b.evalValueFromFile(yamlGetter)),
		},
	}

	return ev.Evaluate(instance)
}

func (b *builder) withValueCache(funcName string, fn eval.Function) eval.Function {
	return func(instance *eval.Instance, args ...interface{}) (interface{}, error) {
		var sargs []string
		for _, arg := range args {
			sargs = append(sargs, fmt.Sprintf("%v", arg))
		}
		key := fmt.Sprintf("%s(%s)", funcName, strings.Join(sargs, ","))
		if v, ok := b.valueCache.Get(key); ok {
			return v, nil
		}
		v, err := fn(instance, args...)
		if err == nil {
			b.valueCache.Set(key, v, cache.DefaultExpiration)
		}
		return v, err
	}
}

func evalCommandShell(_ *eval.Instance, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, errors.New(`expecting at least one argument`)
	}
	command, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf(`expecting string value for command argument`)
	}

	var shellAndArgs []string

	if len(args) > 1 {
		for _, arg := range args[1:] {
			s, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf(`expecting only string value for shell command and arguments`)
			}
			shellAndArgs = append(shellAndArgs, s)
		}
	}
	return valueFromShellCommand(command, shellAndArgs...)
}

func valueFromShellCommand(command string, shellAndArgs ...string) (interface{}, error) {
	log.Debugf("Resolving value from shell command: %s, args [%s]", command, strings.Join(shellAndArgs, ","))

	shellCmd := &compliance.ShellCmd{
		Run: command,
	}
	if len(shellAndArgs) > 0 {
		shellCmd.Shell = &compliance.BinaryCmd{
			Name: shellAndArgs[0],
			Args: shellAndArgs[1:],
		}
	}
	execCommand := shellCmdToBinaryCmd(shellCmd)
	exitCode, stdout, err := runBinaryCmd(execCommand, defaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}
	return stdout, nil
}

func evalCommandExec(_ *eval.Instance, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, errors.New(`expecting at least one argument`)
	}

	var cmdArgs []string

	for _, arg := range args {
		s, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf(`expecting only string values for arguments`)
		}
		cmdArgs = append(cmdArgs, s)
	}

	return valueFromBinaryCommand(cmdArgs[0], cmdArgs[1:]...)
}

func valueFromBinaryCommand(name string, args ...string) (interface{}, error) {
	log.Debugf("Resolving value from command: %s, args [%s]", name, strings.Join(args, ","))
	execCommand := &compliance.BinaryCmd{
		Name: name,
		Args: args,
	}
	exitCode, stdout, err := runBinaryCmd(execCommand, defaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", execCommand, err)
	}
	return stdout, nil
}

func evalProcessFlag(_ *eval.Instance, args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, errors.New(`expecting two arguments`)
	}
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf(`expecting string value for process name argument`)
	}
	flag, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf(`expecting string value for process flag argument`)
	}
	return valueFromProcessFlag(name, flag)
}

func valueFromProcessFlag(name string, flag string) (interface{}, error) {
	log.Debugf("Resolving value from process: %s, flag %s", name, flag)

	processes, err := getProcesses(cacheValidity)
	if err != nil {
		return "", fmt.Errorf("unable to fetch processes: %w", err)
	}

	matchedProcesses := processes.findProcessesByName(name)
	for _, mp := range matchedProcesses {
		flagValues := parseProcessCmdLine(mp.Cmdline)
		flagValue, found := flagValues[flag]
		if !found {
			return false, nil
		}
		if flagValue == "" {
			return true, nil
		}
		return flagValue, nil
	}
	return "", fmt.Errorf("failed to find process: %s", name)
}

func (b *builder) evalValueFromFile(get getter) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for path argument`)
		}

		path = b.NormalizePath(path)

		query, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for query argument`)
		}
		return queryValueFromFile(path, query, get)
	}
}
