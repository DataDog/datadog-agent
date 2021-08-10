// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	cache "github.com/patrickmn/go-cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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
	GetCheckStatus() compliance.CheckStatusList
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

// WithMaxEvents configures default max events per run
func WithMaxEvents(max int) BuilderOption {
	return func(b *builder) error {
		b.maxEventsPerRun = max
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
		b.pathMapper = &pathMapper{
			hostMountPath: hostRootMount,
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

type kubeClient struct {
	dynamic.Interface
	clusterID string
}

func (c *kubeClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return c.Interface.Resource(resource)
}

func (c *kubeClient) ClusterID() (string, error) {
	if c.clusterID != "" {
		return c.clusterID, nil
	}

	resourceDef := c.Resource(schema.GroupVersionResource{
		Resource: "namespaces",
		Version:  "v1",
	})

	resource, err := resourceDef.Get(context.TODO(), "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	c.clusterID = string(resource.GetUID())
	return c.clusterID, nil
}

// WithKubernetesClient allows specific Kubernetes client
func WithKubernetesClient(cli env.KubeClient, clusterID string) BuilderOption {
	return func(b *builder) error {
		b.kubeClient = &kubeClient{Interface: cli, clusterID: clusterID}
		return nil
	}
}

// WithIsLeader allows check runner to know if its a leader instance or not (DCA)
func WithIsLeader(isLeader func() bool) BuilderOption {
	return func(b *builder) error {
		b.isLeaderFunc = isLeader
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
type RuleMatcher func(*compliance.RuleBase) bool

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

// WithNodeLabels configures a builder to use specified Kubernetes node labels
func WithNodeLabels(nodeLabels map[string]string) BuilderOption {
	return func(b *builder) error {
		b.nodeLabels = map[string]string{}
		for k, v := range nodeLabels {
			k, v := hostinfo.LabelPreprocessor(k, v)
			b.nodeLabels[k] = v
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
	return func(r *compliance.RuleBase) bool {
		return r.ID == ruleID
	}
}

// NewBuilder constructs a check builder
func NewBuilder(reporter event.Reporter, options ...BuilderOption) (Builder, error) {
	b := &builder{
		reporter:        reporter,
		checkInterval:   20 * time.Minute,
		maxEventsPerRun: 30,
		etcGroupPath:    "/etc/group",
		status:          newStatus(),
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
	checkInterval   time.Duration
	maxEventsPerRun int

	reporter   event.Reporter
	valueCache *cache.Cache

	hostname     string
	pathMapper   *pathMapper
	etcGroupPath string
	nodeLabels   map[string]string

	suiteMatcher SuiteMatcher
	ruleMatcher  RuleMatcher

	dockerClient env.DockerClient
	auditClient  env.AuditClient
	kubeClient   *kubeClient
	isLeaderFunc func() bool

	status *status
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

	if b.suiteMatcher != nil {
		if b.suiteMatcher(&suite.Meta) {
			log.Infof("%s/%s: matched suite in %s", suite.Meta.Name, suite.Meta.Version, file)
		} else {
			log.Tracef("%s/%s: skipped suite in %s", suite.Meta.Name, suite.Meta.Version, file)
			return nil
		}
	}

	log.Infof("%s/%s: loading suite from %s", suite.Meta.Name, suite.Meta.Version, file)

	matchedCount := 0
	for _, r := range suite.Rules {
		fmt.Printf("Rule: %+v\n", r)

		if b.ruleMatcher != nil {
			if b.ruleMatcher(&r.RuleBase) {
				log.Infof("%s/%s: matched rule %s in %s", suite.Meta.Name, suite.Meta.Version, r.ID, file)
			} else {
				log.Tracef("%s/%s: skipped rule %s in %s", suite.Meta.Name, suite.Meta.Version, r.ID, file)
				continue
			}
		}
		matchedCount++

		if len(r.Resources) == 0 {
			log.Infof("%s/%s: skipped rule %s - no configured resources", suite.Meta.Name, suite.Meta.Version, r.ID)
			continue
		}

		log.Debugf("%s/%s: loading rule %s", suite.Meta.Name, suite.Meta.Version, r.ID)
		check, err := b.checkFromRule(&suite.Meta, &r)

		if err != nil {
			if err != ErrRuleDoesNotApply {
				log.Warnf("%s/%s: failed to load rule %s: %v", suite.Meta.Name, suite.Meta.Version, r.ID, err)
			}
			log.Infof("%s/%s: skipped rule %s - does not apply to this system", suite.Meta.Name, suite.Meta.Version, r.ID)
		}

		if b.status != nil {
			b.status.addCheck(&compliance.CheckStatus{
				RuleID:      r.ID,
				Description: r.Description,
				Name:        compliance.CheckName(r.ID, r.Description),
				Framework:   suite.Meta.Framework,
				Source:      suite.Meta.Source,
				Version:     suite.Meta.Version,
				InitError:   err,
			})
		}
		ok := onCheck(&r.RuleBase, check, err)
		if !ok {
			log.Infof("%s/%s: stopping rule enumeration", suite.Meta.Name, suite.Meta.Version)
			return err
		}
	}

	for _, r := range suite.RegoRules {
		if b.ruleMatcher != nil {
			if b.ruleMatcher(&r.RuleBase) {
				log.Infof("%s/%s: matched rule %s in %s", suite.Meta.Name, suite.Meta.Version, r.ID, file)
			} else {
				log.Tracef("%s/%s: skipped rule %s in %s", suite.Meta.Name, suite.Meta.Version, r.ID, file)
				continue
			}
		}
		matchedCount++

		if len(r.Resources) == 0 {
			log.Infof("%s/%s: skipped rule %s - no configured resources", suite.Meta.Name, suite.Meta.Version, r.ID)
			continue
		}

		log.Debugf("%s/%s: loading rule %s", suite.Meta.Name, suite.Meta.Version, r.ID)
		check, err := b.checkFromRegoRule(&suite.Meta, &r)

		if err != nil {
			if err != ErrRuleDoesNotApply {
				log.Warnf("%s/%s: failed to load rule %s: %v", suite.Meta.Name, suite.Meta.Version, r.ID, err)
			}
			log.Infof("%s/%s: skipped rule %s - does not apply to this system", suite.Meta.Name, suite.Meta.Version, r.ID)
		}

		if b.status != nil {
			b.status.addCheck(&compliance.CheckStatus{
				RuleID:      r.ID,
				Description: r.Description,
				Name:        compliance.CheckName(r.ID, r.Description),
				Framework:   suite.Meta.Framework,
				Source:      suite.Meta.Source,
				Version:     suite.Meta.Version,
				InitError:   err,
			})
		}
		ok := onCheck(&r.RuleBase, check, err)
		if !ok {
			log.Infof("%s/%s: stopping rule enumeration", suite.Meta.Name, suite.Meta.Version)
			return err
		}
	}

	if b.ruleMatcher != nil && matchedCount == 0 {
		log.Infof("%s/%s: no rules matched", suite.Meta.Name, suite.Meta.Version)
	}

	return nil
}

func (b *builder) GetCheckStatus() compliance.CheckStatusList {
	if b.status != nil {
		return b.status.getChecksStatus()
	}
	return compliance.CheckStatusList{}
}

func (b *builder) checkFromRule(meta *compliance.SuiteMeta, rule *compliance.Rule) (compliance.Check, error) {
	ruleScope, err := getRuleScope(meta, rule.Scope)
	if err != nil {
		return nil, err
	}

	eligible, err := b.hostMatcher(ruleScope, rule.ID, rule.HostSelector)
	if err != nil {
		return nil, err
	}

	if !eligible {
		log.Debugf("rule %s/%s discarded by hostMatcher", meta.Framework, rule.ID)
		return nil, ErrRuleDoesNotApply
	}

	resourceReporter := b.getRuleResourceReporter(ruleScope, rule.ResourceType)
	return b.newCheck(meta, ruleScope, rule, resourceReporter)
}

func (b *builder) checkFromRegoRule(meta *compliance.SuiteMeta, rule *compliance.RegoRule) (compliance.Check, error) {
	ruleScope, err := getRuleScope(meta, rule.Scope)
	if err != nil {
		return nil, err
	}

	eligible, err := b.hostMatcher(ruleScope, rule.ID, rule.HostSelector)
	if err != nil {
		return nil, err
	}

	if !eligible {
		log.Debugf("rule %s/%s discarded by hostMatcher", meta.Framework, rule.ID)
		return nil, ErrRuleDoesNotApply
	}

	resourceReporter := b.getRuleResourceReporter(ruleScope, rule.ResourceType)
	return b.newRegoCheck(meta, ruleScope, rule, resourceReporter)
}

func getRuleScope(meta *compliance.SuiteMeta, scopeList compliance.RuleScopeList) (compliance.RuleScope, error) {
	switch {
	case scopeList.Includes(compliance.DockerScope):
		return compliance.DockerScope, nil
	case scopeList.Includes(compliance.KubernetesNodeScope):
		return compliance.KubernetesNodeScope, nil
	case scopeList.Includes(compliance.KubernetesClusterScope):
		return compliance.KubernetesClusterScope, nil
	default:
		return "", ErrRuleScopeNotSupported
	}
}

func (b *builder) kubeResourceReporter(resourceType string) resourceReporter {
	return func(report *compliance.Report) compliance.ReportResource {
		var clusterID string
		var err error

		if b.kubeClient != nil {
			clusterID, err = b.kubeClient.ClusterID()
			if err != nil {
				log.Debugf("failed to retrieve cluster id, defaulting to hostname")
			}
		}

		if clusterID == "" {
			clusterID = b.Hostname()
		}

		if !report.Aggregated && resourceType == "" && strings.HasPrefix(report.Resource.Type, "kube_") {
			return compliance.ReportResource{
				ID:   clusterID + "_" + report.Resource.ID,
				Type: report.Resource.Type,
			}
		}

		return compliance.ReportResource{
			ID:   clusterID + "_" + resourceType,
			Type: resourceType,
		}
	}
}

func (b *builder) getRuleResourceReporter(scope compliance.RuleScope, resourceType string) resourceReporter {
	switch scope {
	case compliance.DockerScope:
		return func(report *compliance.Report) compliance.ReportResource {
			if !report.Aggregated && resourceType == "" && strings.HasPrefix(report.Resource.Type, "docker_") {
				return compliance.ReportResource{
					ID:   b.Hostname() + "_" + report.Resource.ID,
					Type: report.Resource.Type,
				}
			}

			resourceType := resourceType
			if resourceType == "" {
				resourceType = "docker_daemon"
			}

			return compliance.ReportResource{
				ID:   b.Hostname() + "_daemon",
				Type: resourceType,
			}
		}

	case compliance.KubernetesNodeScope:
		return b.kubeResourceReporter("kubernetes_node")

	case compliance.KubernetesClusterScope:
		return b.kubeResourceReporter("kubernetes_cluster")

	default:
		return func(report *compliance.Report) compliance.ReportResource {
			return compliance.ReportResource{
				ID:   b.Hostname(),
				Type: string(scope),
			}
		}
	}
}

func (b *builder) hostMatcher(scope compliance.RuleScope, ruleID string, hostSelector string) (bool, error) {
	switch scope {
	case compliance.DockerScope:
		if b.dockerClient == nil {
			log.Infof("rule %s skipped - not running in a docker environment", ruleID)
			return false, nil
		}
	case compliance.KubernetesClusterScope:
		if b.kubeClient == nil {
			log.Infof("rule %s skipped - not running as Cluster Agent", ruleID)
			return false, nil
		}
	case compliance.KubernetesNodeScope:
		if config.IsKubernetes() {
			return b.isKubernetesNodeEligible(hostSelector)
		}
		log.Infof("rule %s skipped - not running on a Kubernetes node", ruleID)
		return false, nil
	}

	return true, nil
}

func (b *builder) isKubernetesNodeEligible(hostSelector string) (bool, error) {
	if hostSelector == "" {
		return true, nil
	}

	expr, err := eval.ParseExpression(hostSelector)
	if err != nil {
		return false, err
	}

	nodeInstance := eval.NewInstance(
		eval.VarMap{
			"node.labels": b.nodeLabelKeys(),
		},
		eval.FunctionMap{
			"node.hasLabel": b.nodeHasLabel,
			"node.label":    b.nodeLabel,
		},
	)

	result, err := expr.Evaluate(nodeInstance)
	if err != nil {
		return false, err
	}

	eligible, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("hostSelector %q does not evaluate to a boolean value", hostSelector)
	}

	return eligible, nil
}

func (b *builder) getNodeLabel(args ...interface{}) (string, bool, error) {
	if len(args) == 0 {
		return "", false, errors.New(`expecting one argument for label`)
	}
	label, ok := args[0].(string)
	if !ok {
		return "", false, fmt.Errorf(`expecting string value for label argument`)
	}
	if b.nodeLabels == nil {
		return "", false, nil
	}
	v, ok := b.nodeLabels[label]
	return v, ok, nil
}

func (b *builder) nodeHasLabel(_ eval.Instance, args ...interface{}) (interface{}, error) {
	_, ok, err := b.getNodeLabel(args...)
	return ok, err
}

func (b *builder) nodeLabel(_ eval.Instance, args ...interface{}) (interface{}, error) {
	v, _, err := b.getNodeLabel(args...)
	return v, err
}

func (b *builder) nodeLabelKeys() []string {
	var keys []string
	for k := range b.nodeLabels {
		keys = append(keys, k)
	}
	return keys
}

func (b *builder) newCheck(meta *compliance.SuiteMeta, ruleScope compliance.RuleScope, rule *compliance.Rule, handler resourceReporter) (compliance.Check, error) {
	checkable, err := newResourceCheckList(b, rule.ID, rule.Resources)

	if err != nil {
		return nil, err
	}

	var notify eventNotify
	if b.status != nil {
		notify = b.status.updateCheck
	}

	// We capture err as configuration error but do not prevent check creation
	return &complianceCheck{
		Env: b,

		ruleID:      rule.ID,
		description: rule.Description,
		interval:    b.checkInterval,

		suiteMeta: meta,

		resourceHandler: handler,
		scope:           ruleScope,
		checkable:       checkable,

		eventNotify: notify,
	}, nil
}

func (b *builder) newRegoCheck(meta *compliance.SuiteMeta, ruleScope compliance.RuleScope, rule *compliance.RegoRule, handler resourceReporter) (compliance.Check, error) {
	check := &regoCheck{
		Env: b,

		ruleID:      rule.ID,
		description: rule.Description,
		interval:    b.checkInterval,

		suiteMeta: meta,

		resourceHandler: handler,
		scope:           ruleScope,

		resources: rule.Resources,
	}

	if err := check.compileQuery(rule.Module, rule.Query); err != nil {
		return nil, err
	}

	return check, nil
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

func (b *builder) MaxEventsPerRun() int {
	return b.maxEventsPerRun
}

func (b *builder) NormalizeToHostRoot(path string) string {
	if b.pathMapper == nil {
		return path
	}
	return b.pathMapper.normalizeToHostRoot(path)
}

func (b *builder) RelativeToHostRoot(path string) string {
	if b.pathMapper == nil {
		return path
	}
	return b.pathMapper.relativeToHostRoot(path)
}

func (b *builder) IsLeader() bool {
	if b.isLeaderFunc != nil {
		return b.isLeaderFunc()
	}
	return true
}

func (b *builder) EvaluateFromCache(ev eval.Evaluatable) (interface{}, error) {
	instance := eval.NewInstance(
		nil,
		eval.FunctionMap{
			builderFuncShell:       b.withValueCache(builderFuncShell, evalCommandShell),
			builderFuncExec:        b.withValueCache(builderFuncExec, evalCommandExec),
			builderFuncProcessFlag: b.withValueCache(builderFuncProcessFlag, evalProcessFlag),
			builderFuncJSON:        b.withValueCache(builderFuncJSON, b.evalValueFromFile(jsonGetter)),
			builderFuncYAML:        b.withValueCache(builderFuncYAML, b.evalValueFromFile(yamlGetter)),
		},
	)

	return ev.Evaluate(instance)
}

func (b *builder) withValueCache(funcName string, fn eval.Function) eval.Function {
	return func(instance eval.Instance, args ...interface{}) (interface{}, error) {
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

func evalCommandShell(_ eval.Instance, args ...interface{}) (interface{}, error) {
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

func evalCommandExec(_ eval.Instance, args ...interface{}) (interface{}, error) {
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

func evalProcessFlag(_ eval.Instance, args ...interface{}) (interface{}, error) {
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
		return flagValues[flag], nil
	}
	return "", fmt.Errorf("failed to find process: %s", name)
}

func (b *builder) evalValueFromFile(get getter) eval.Function {
	return func(_ eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for path argument`)
		}

		path = b.NormalizeToHostRoot(path)

		query, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for query argument`)
		}
		return queryValueFromFile(path, query, get)
	}
}
