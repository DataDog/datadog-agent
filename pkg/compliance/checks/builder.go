// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checks implements Compliance Agent checks
package checks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	cache "github.com/patrickmn/go-cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/audit"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/file"
	commandutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/command"
	dockerutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/docker"
	fileutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/file"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
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

// WithStatsd configures the statsd client for compliance metrics
func WithStatsd(client statsd.ClientInterface) BuilderOption {
	return func(b *builder) error {
		b.statsdClient = client
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

// WithConfigDir configures the configuration directory
func WithConfigDir(configDir string) BuilderOption {
	return func(b *builder) error {
		b.configDir = configDir
		return nil
	}
}

// WithHostRootMount defines host root filesystem mount location
func WithHostRootMount(hostRootMount string) BuilderOption {
	return func(b *builder) error {
		if hostRootMount == "" {
			hostRootMount = "/"
		}
		log.Infof("Host root filesystem will be remapped to %s", hostRootMount)
		b.pathMapper = fileutils.NewPathMapper(
			hostRootMount,
		)
		return nil
	}
}

// WithDocker configures using docker
func WithDocker() BuilderOption {
	return func(b *builder) error {
		cli, err := dockerutils.NewDockerClient()
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
		cli, err := audit.NewAuditClient()
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

	ctx, cancel := context.WithTimeout(context.Background(), compliance.DefaultTimeout)
	defer cancel()
	resource, err := resourceDef.Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	c.clusterID = string(resource.GetUID())
	return c.clusterID, nil
}

// WithKubernetesClient allows specific Kubernetes client
func WithKubernetesClient(cli dynamic.Interface, clusterID string) BuilderOption {
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
type RuleMatcher func(*compliance.RuleCommon) bool

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

// WithRegoInput configures a builder to provide rego input based on the content
// of a file instead of the current environment
func WithRegoInput(regoInputPath string) BuilderOption {
	return func(b *builder) error {
		content, err := os.ReadFile(regoInputPath)
		if err != nil {
			return err
		}
		return json.Unmarshal(content, &b.regoInputOverride)
	}
}

// WithRegoInputDumpPath configures a builder to dump the rego input to the provided file path
func WithRegoInputDumpPath(regoInputDumpPath string) BuilderOption {
	return func(b *builder) error {
		b.regoInputDumpPath = regoInputDumpPath
		return nil
	}
}

// WithRegoEvalSkip configures a builder to skip the rego evaluation, while still building the input
func WithRegoEvalSkip(regoEvalSkip bool) BuilderOption {
	return func(b *builder) error {
		b.regoEvalSkip = regoEvalSkip
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
	return func(r *compliance.RuleCommon) bool {
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

	reporter     event.Reporter
	valueCache   *cache.Cache
	statsdClient statsd.ClientInterface

	hostname     string
	pathMapper   *fileutils.PathMapper
	etcGroupPath string
	configDir    string

	suiteMatcher SuiteMatcher
	ruleMatcher  RuleMatcher

	dockerClient env.DockerClient
	auditClient  env.AuditClient
	kubeClient   *kubeClient
	isLeaderFunc func() bool

	regoInputOverride map[string]eval.RegoInputMap
	regoInputDumpPath string
	regoEvalSkip      bool

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

func (b *builder) checkMatchingRule(file string, suite *compliance.Suite, rule compliance.Rule) bool {
	ruleCommon := rule.Common()
	if b.ruleMatcher != nil {
		if b.ruleMatcher(ruleCommon) {
			log.Infof("%s/%s: matched rule %s in %s", suite.Meta.Name, suite.Meta.Version, ruleCommon.ID, file)
		} else {
			log.Tracef("%s/%s: skipped rule %s in %s", suite.Meta.Name, suite.Meta.Version, ruleCommon.ID, file)
			return false
		}
	}

	if rule.ResourceCount() == 0 {
		log.Infof("%s/%s: skipped rule %s - no configured resources", suite.Meta.Name, suite.Meta.Version, ruleCommon.ID)
		return false
	}

	return true
}

func (b *builder) addCheckAndRun(suite *compliance.Suite, r *compliance.RuleCommon, check compliance.Check, onCheck compliance.CheckVisitor, initErr error) error {
	if b.status != nil {
		b.status.addCheck(&compliance.CheckStatus{
			RuleID:      r.ID,
			Description: r.Description,
			Name:        compliance.CheckName(r.ID, r.Description),
			Framework:   suite.Meta.Framework,
			Source:      suite.Meta.Source,
			Version:     suite.Meta.Version,
			InitError:   initErr,
		})
	}
	ok := onCheck(r, check, initErr)
	if !ok {
		log.Infof("%s/%s: stopping rule enumeration", suite.Meta.Name, suite.Meta.Version)
		return initErr
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
	for _, r := range suite.RegoRules {
		if b.checkMatchingRule(file, suite, &r) {
			matchedCount++
		} else {
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

		if err := b.addCheckAndRun(suite, r.Common(), check, onCheck, err); err != nil {
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

func (b *builder) StatsdClient() statsd.ClientInterface {
	return b.statsdClient
}

func (b *builder) checkFromRegoRule(meta *compliance.SuiteMeta, rule *compliance.RegoRule) (compliance.Check, error) {
	// skip rules with Xccdf input if compliance_config.xccdf.enabled is no optin
	if !config.Datadog.GetBool("compliance_config.xccdf.enabled") && rule.HasResourceKind(compliance.KindXccdf) {
		return nil, ErrRuleDoesNotApply
	}

	ruleScope, err := getRuleScope(meta, rule.Scope)
	if err != nil {
		return nil, err
	}

	// skip the scope checks if rego inputs were provided via CLI
	if b.regoInputOverride == nil {
		switch ruleScope {
		case compliance.DockerScope:
			if rule.SkipOnK8s && config.IsKubernetes() {
				log.Infof("rule %s skipped - running on a Kubernetes environment", rule.ID)
				return nil, ErrRuleDoesNotApply
			}

			if b.dockerClient == nil {
				log.Infof("rule %s skipped - not running in a docker environment", rule.ID)
				return nil, ErrRuleDoesNotApply
			}
		case compliance.KubernetesClusterScope:
			if b.kubeClient == nil {
				log.Infof("rule %s skipped - not running as Cluster Agent", rule.ID)
				return nil, ErrRuleDoesNotApply
			}
		case compliance.KubernetesNodeScope:
			if !config.IsKubernetes() {
				log.Infof("rule %s skipped - not running on a Kubernetes node", rule.ID)
				return nil, ErrRuleDoesNotApply
			}
		}
	}

	regoCheck := rego.NewCheck(rule)
	if err := regoCheck.CompileRule(rule, ruleScope, meta); err != nil {
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

		resourceHandler: fallthroughReporter,
		scope:           ruleScope,
		checkable:       regoCheck,

		eventNotify: notify,
	}, nil
}

func fallthroughReporter(report *compliance.Report) compliance.ReportResource {
	return report.Resource
}

func getRuleScope(meta *compliance.SuiteMeta, scopeList compliance.RuleScopeList) (compliance.RuleScope, error) {
	switch {
	case scopeList.Includes(compliance.DockerScope):
		return compliance.DockerScope, nil
	case scopeList.Includes(compliance.KubernetesNodeScope):
		return compliance.KubernetesNodeScope, nil
	case scopeList.Includes(compliance.KubernetesClusterScope):
		return compliance.KubernetesClusterScope, nil
	case scopeList.Includes(compliance.Unscoped):
		return compliance.Unscoped, nil
	default:
		return "", ErrRuleScopeNotSupported
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

func (b *builder) ProvidedInput(ruleID string) eval.RegoInputMap {
	return b.regoInputOverride[ruleID]
}

func (b *builder) DumpInputPath() string {
	return b.regoInputDumpPath
}

func (b *builder) ShouldSkipRegoEval() bool {
	return b.regoEvalSkip
}

func (b *builder) Hostname() string {
	return b.hostname
}

func (b *builder) ConfigDir() string {
	return b.configDir
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
	return b.pathMapper.NormalizeToHostRoot(path)
}

func (b *builder) RelativeToHostRoot(path string) string {
	if b.pathMapper == nil {
		return path
	}
	return b.pathMapper.RelativeToHostRoot(path)
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
			builderFuncJSON:        b.withValueCache(builderFuncJSON, b.evalValueFromFile(fileutils.JSONGetter)),
			builderFuncYAML:        b.withValueCache(builderFuncYAML, b.evalValueFromFile(fileutils.YAMLGetter)),
		},
		nil,
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
	execCommand := commandutils.ShellCmdToBinaryCmd(shellCmd)
	exitCode, stdout, err := commandutils.RunBinaryCmd(execCommand, compliance.DefaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}
	return stdout, nil
}

func valueFromBinaryCommand(name string, args ...string) (interface{}, error) {
	log.Debugf("Resolving value from command: %s, args [%s]", name, strings.Join(args, ","))
	execCommand := &compliance.BinaryCmd{
		Name: name,
		Args: args,
	}
	exitCode, stdout, err := commandutils.RunBinaryCmd(execCommand, compliance.DefaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", execCommand, err)
	}
	return stdout, nil
}

func evalCommandShell(_ eval.Instance, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, errors.New(`expecting at least one argument`)
	}
	cmd, ok := args[0].(string)
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

	return valueFromShellCommand(cmd, shellAndArgs...)
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
	return processutils.ValueFromProcessFlag(name, flag)
}

func (b *builder) evalValueFromFile(get fileutils.Getter) eval.Function {
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
		return file.QueryValueFromFile(path, query, get)
	}
}
