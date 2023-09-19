// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/version"
	"gopkg.in/yaml.v2"
)

// Evaluator is a string representing the type of evaluator that produced the
// event.
type Evaluator string

const (
	// RegoEvaluator uses the rego engine to evaluate a check.
	RegoEvaluator Evaluator = "rego"
	// XCCDFEvaluator uses OpenSCAP to evaluate a check.
	XCCDFEvaluator Evaluator = "xccdf"
)

// CheckResult lists the different states of a check result.
type CheckResult string

const (
	// CheckPassed is used to report successful result of a rule check
	// (condition passed)
	CheckPassed CheckResult = "passed"
	// CheckFailed is used to report unsuccessful result of a rule check
	// (condition failed)
	CheckFailed CheckResult = "failed"
	// CheckError is used to report result of a rule check that resulted in an
	// error (unable to evaluate condition)
	CheckError CheckResult = "error"
	// CheckSkipped is used to report result of a rule that is being skipped.
	CheckSkipped CheckResult = "skipped"
)

// CheckStatus is used to store the last current status of each rule inside
// our compliance agent.
type CheckStatus struct {
	RuleID      string
	Name        string
	Description string
	Version     string
	Framework   string
	Source      string
	InitError   error
	LastEvent   *CheckEvent
}

// CheckContainerMeta holds metadata related to the container that has been checked.
type CheckContainerMeta struct {
	ContainerID string `json:"container_id"`
	ImageID     string `json:"image_id"`
	ImageName   string `json:"image_name"`
	ImageTag    string `json:"image_tag"`
}

// CheckEvent is the data structure sent to the backend as a result of a rule
// evaluation.
type CheckEvent struct {
	AgentVersion string                 `json:"agent_version,omitempty"`
	RuleID       string                 `json:"agent_rule_id,omitempty"`
	RuleVersion  int                    `json:"agent_rule_version,omitempty"`
	FrameworkID  string                 `json:"agent_framework_id,omitempty"`
	Evaluator    Evaluator              `json:"evaluator,omitempty"`
	ExpireAt     time.Time              `json:"expire_at,omitempty"`
	Result       CheckResult            `json:"result,omitempty"`
	ResourceType string                 `json:"resource_type,omitempty"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	Container    *CheckContainerMeta    `json:"container,omitempty"`
	Tags         []string               `json:"tags"`
	Data         map[string]interface{} `json:"data"`

	errReason error `json:"-"`
}

// ResourceLog is the data structure holding a resource configuration data.
type ResourceLog struct {
	AgentVersion string      `json:"agent_version,omitempty"`
	ExpireAt     time.Time   `json:"expire_at,omitempty"`
	ResourceType string      `json:"resource_type,omitempty"`
	ResourceID   string      `json:"resource_id,omitempty"`
	ResourceData interface{} `json:"resource_data,omitempty"`
	Tags         []string    `json:"tags"`
}

func (e *CheckEvent) String() string {
	s := fmt.Sprintf("%s:%s result=%s", e.FrameworkID, e.RuleID, e.Result)
	if e.ResourceID != "" {
		s += fmt.Sprintf(" resource=%s:%s", e.ResourceType, e.ResourceID)
	}
	if e.Result == CheckError {
		s += fmt.Sprintf(" error=%s", e.errReason)
	} else {
		s += fmt.Sprintf(" data=%v", e.Data)
	}
	return s
}

// NewCheckError returns a CheckEvent with error status and associated error
// reason.
func NewCheckError(
	evaluator Evaluator,
	errReason error,
	resourceID,
	resourceType string,
	rule *Rule,
	benchmark *Benchmark,
) *CheckEvent {
	expireAt := time.Now().Add(1 * time.Hour).UTC().Truncate(1 * time.Second)
	return &CheckEvent{
		AgentVersion: version.AgentVersion,
		RuleID:       rule.ID,
		FrameworkID:  benchmark.FrameworkID,
		ResourceID:   resourceID,
		ResourceType: resourceType,
		ExpireAt:     expireAt,
		Evaluator:    evaluator,
		Result:       CheckError,
		Data:         map[string]interface{}{"error": errReason.Error()},

		errReason: errReason,
	}
}

// NewCheckEvent returns a CheckEvent with given status.
func NewCheckEvent(
	evaluator Evaluator,
	result CheckResult,
	data map[string]interface{},
	resourceID,
	resourceType string,
	rule *Rule,
	benchmark *Benchmark,
) *CheckEvent {
	expireAt := time.Now().Add(1 * time.Hour).UTC().Truncate(1 * time.Second)
	return &CheckEvent{
		AgentVersion: version.AgentVersion,
		RuleID:       rule.ID,
		FrameworkID:  benchmark.FrameworkID,
		ResourceID:   resourceID,
		ResourceType: resourceType,
		ExpireAt:     expireAt,
		Evaluator:    evaluator,
		Result:       result,
		Data:         data,
	}
}

// NewCheckSkipped returns a CheckEvent with skipped status.
func NewCheckSkipped(
	evaluator Evaluator,
	skipReason error,
	resourceID,
	resourceType string,
	rule *Rule,
	benchmark *Benchmark,
) *CheckEvent {
	expireAt := time.Now().Add(1 * time.Hour).UTC().Truncate(1 * time.Second)
	return &CheckEvent{
		AgentVersion: version.AgentVersion,
		RuleID:       rule.ID,
		FrameworkID:  benchmark.FrameworkID,
		ExpireAt:     expireAt,
		Evaluator:    evaluator,
		Result:       CheckSkipped,
		Data:         map[string]interface{}{"error": skipReason.Error()},
	}
}

// NewResourceLog returns a ResourceLog.
func NewResourceLog(resourceID, resourceType string, resource interface{}) *ResourceLog {
	expireAt := time.Now().Add(1 * time.Hour).UTC().Truncate(1 * time.Second)
	return &ResourceLog{
		AgentVersion: version.AgentVersion,
		ExpireAt:     expireAt,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceData: resource,
	}
}

// ErrIncompatibleEnvironment is returns by the resolver to signal that the
// given rule's inputs are not resolvable in the current environment.
var ErrIncompatibleEnvironment = errors.New("environment not compatible this type of input")

// CheckEventFromError wraps any error into a correct CheckEvent, detecting if
// the underlying error should be marked as skipped or not.
func CheckEventFromError(evaluator Evaluator, rule *Rule, benchmark *Benchmark, err error) *CheckEvent {
	if errors.Is(err, ErrIncompatibleEnvironment) {
		return NewCheckSkipped(evaluator, fmt.Errorf("skipping input resolution for rule=%s: %w", rule.ID, err), "", "", rule, benchmark)
	}
	return NewCheckError(evaluator, fmt.Errorf("input resolution error for rule=%s: %w", rule.ID, err), "", "", rule, benchmark)
}

// RuleScope defines the different context in which the rule is allowed to run.
type RuleScope string

const (
	// Unscoped scope used when no particular scope is required
	Unscoped RuleScope = "none"
	// DockerScope used for rules requiring a Docker daemon running.
	DockerScope RuleScope = "docker"
	// KubernetesNodeScope used for rules requireing a kubelet process running.
	KubernetesNodeScope RuleScope = "kubernetesNode"
	// KubernetesClusterScope used for rules requireing a kube-apiserver process running.
	KubernetesClusterScope RuleScope = "kubernetesCluster"
)

// RuleFilter defines a function type that can be used to filter rules.
type RuleFilter func(*Rule) bool

// Rule defines a list of inputs against which we can evaluate properties. It
// also holds all metadata associated with the rule.
type Rule struct {
	ID          string       `yaml:"id" json:"id"`
	Description string       `yaml:"description,omitempty" json:"description,omitempty"`
	SkipOnK8s   bool         `yaml:"skipOnKubernetes,omitempty" json:"skipOnKubernetes,omitempty"`
	Module      string       `yaml:"module,omitempty" json:"module,omitempty"`
	Scopes      []RuleScope  `yaml:"scope,omitempty" json:"scope,omitempty"`
	InputSpecs  []*InputSpec `yaml:"input,omitempty" json:"input,omitempty"`
	Imports     []string     `yaml:"imports,omitempty" json:"imports,omitempty"`
	Period      string       `yaml:"period,omitempty" json:"period,omitempty"`
	Filters     []string     `yaml:"filters,omitempty" json:"filters,omitempty"`
}

type (
	// InputSpec is a union type that holds the description of a set of inputs
	// to be gathered typically by a Resolver.
	InputSpec struct {
		File          *InputSpecFile          `yaml:"file,omitempty" json:"file,omitempty"`
		Process       *InputSpecProcess       `yaml:"process,omitempty" json:"process,omitempty"`
		Group         *InputSpecGroup         `yaml:"group,omitempty" json:"group,omitempty"`
		Audit         *InputSpecAudit         `yaml:"audit,omitempty" json:"audit,omitempty"`
		Docker        *InputSpecDocker        `yaml:"docker,omitempty" json:"docker,omitempty"`
		KubeApiserver *InputSpecKubeapiserver `yaml:"kubeApiserver,omitempty" json:"kubeApiserver,omitempty"`
		Package       *InputSpecPackage       `yaml:"package,omitempty" json:"package,omitempty"`
		XCCDF         *InputSpecXCCDF         `yaml:"xccdf,omitempty" json:"xccdf,omitempty"`
		Constants     *InputSpecConstants     `yaml:"constants,omitempty" json:"constants,omitempty"`

		TagName string `yaml:"tag,omitempty" json:"tag,omitempty"`
		Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	}

	// InputSpecFile describes the spec to resolve file informations.
	InputSpecFile struct {
		Path   string `yaml:"path" json:"path"`
		Glob   string `yaml:"glob" json:"glob"`
		Parser string `yaml:"parser,omitempty" json:"parser,omitempty"`
	}

	// InputSpecProcess describes the spec to resolve process informations.
	InputSpecProcess struct {
		Name string   `yaml:"name" json:"name"`
		Envs []string `yaml:"envs,omitempty" json:"envs,omitempty"`
	}

	// InputSpecGroup describes the spec to resolve a unix group informations.
	InputSpecGroup struct {
		Name string `yaml:"name" json:"name"`
	}

	// InputSpecAudit describes the spec to resolve a Linux Audit informations.
	InputSpecAudit struct {
		Path string `yaml:"path" json:"path"`
	}

	// InputSpecDocker describes the spec to resolve a Docker resource informations.
	InputSpecDocker struct {
		Kind string `yaml:"kind" json:"kind"`
	}

	// InputSpecKubeapiserver describes the spec to resolve a Kubernetes
	// resource information from from kube-apiserver.
	InputSpecKubeapiserver struct {
		Kind          string `yaml:"kind" json:"kind"`
		Version       string `yaml:"version,omitempty" json:"version,omitempty"`
		Group         string `yaml:"group,omitempty" json:"group,omitempty"`
		Namespace     string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
		LabelSelector string `yaml:"labelSelector,omitempty" json:"labelSelector,omitempty"`
		FieldSelector string `yaml:"fieldSelector,omitempty" json:"fieldSelector,omitempty"`
		APIRequest    struct {
			Verb         string `yaml:"verb" json:"verb"`
			ResourceName string `yaml:"resourceName,omitempty" json:"resourceName,omitempty"`
		} `yaml:"apiRequest" json:"apiRequest"`
	}

	// InputSpecPackage defines the names of the software packages that need
	// to be resolved
	InputSpecPackage struct {
		Names []string `yaml:"names" json:"names"`
	}

	// InputSpecXCCDF describes the spec to resolve a XCCDF evaluation result.
	InputSpecXCCDF struct {
		Name    string   `yaml:"name" json:"name"`
		Profile string   `yaml:"profile" json:"profile"`
		Rule    string   `yaml:"rule" json:"rule"`
		Rules   []string `yaml:"rules,omitempty" json:"rules,omitempty"`
	}

	// InputSpecConstants can be used to pass constants data to the evaluator.
	InputSpecConstants map[string]interface{}
)

// ResolvingContext is part of the resolved inputs data that should be passed
// as the "context" field in the rego evaluator input. Note that because of the
// way rego bails when dereferencing an undefined key, we do not mark any json
// tag as "omitempty".
type ResolvingContext struct {
	RuleID            string                `json:"ruleID"`
	Hostname          string                `json:"hostname"`
	KubernetesCluster string                `json:"kubernetes_cluster"`
	ContainerID       string                `json:"container_id"`
	InputSpecs        map[string]*InputSpec `json:"input"`
}

// ResolvedInputs is the generic data structure that is returned by a Resolver and
// passed to the rego evaluator.
//
// Ideally if Go did support inline JSON struct tag, this type would be:
// see https://github.com/golang/go/issues/6213
//
//	struct {
//		Context  *ResolvingContext      `json:"context"`
//		Resolved map[string]interface{} `json:",inline"`
//	}
type ResolvedInputs map[string]interface{}

// GetContext returns the ResolvingContext associated with this resolved
// inputs.
func (r ResolvedInputs) GetContext() *ResolvingContext {
	c := r["context"].(ResolvingContext)
	return &c
}

// NewResolvedInputs builds a ResolvedInputs map from the given resolving
// context and generic resolved data.
func NewResolvedInputs(resolvingContext ResolvingContext, resolved map[string]interface{}) (ResolvedInputs, error) {
	ri := make(ResolvedInputs, len(resolved)+1)
	for k, v := range resolved {
		if k == "context" {
			return nil, fmt.Errorf("NewResolvedInputs: \"context\" is a reserved keyword")
		}
		ri[k] = v
	}
	ri["context"] = resolvingContext
	return ri, nil
}

// Benchmark represents a set of rules that have a common identity, typically
// part of the same framework. It holds metadata that are shared between these
// rules. Rules of a same Benchmark are typically run together.
type Benchmark struct {
	dirname string

	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	FrameworkID string   `yaml:"framework,omitempty" json:"framework,omitempty"`
	Version     string   `yaml:"version,omitempty" json:"version,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Rules       []*Rule  `yaml:"rules,omitempty" json:"rules,omitempty"`
	Source      string   `yaml:"-" json:"-"`
	Schema      struct {
		Version string `yaml:"version" json:"version"`
	} `yaml:"schema,omitempty" json:"schema,omitempty"`
}

// IsRego returns true if the rule is a rego rule.
func (r *Rule) IsRego() bool {
	return !r.IsXCCDF()
}

// IsXCCDF returns true if the rule is a XCCDF / OpenSCAP rule.
func (r *Rule) IsXCCDF() bool {
	return len(r.InputSpecs) == 1 && r.InputSpecs[0].XCCDF != nil
}

// HasScope tests if the rule has the given scope.
func (r *Rule) HasScope(scope RuleScope) bool {
	for _, s := range r.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Valid is a validation check required for InputSpec to be executed.
func (i *InputSpec) Valid() error {
	// NOTE(jinroh): the current semantics allow to specify the result type as
	// an "array". Here we enforce that the specified result type is
	// constrained to a specific input type.
	if i.KubeApiserver != nil || i.Docker != nil || i.Audit != nil {
		if i.Type != "array" {
			return fmt.Errorf("input of types kubeApiserver docker and audit have to be arrays")
		}
	} else if i.Type == "array" {
		if i.File == nil {
			return fmt.Errorf("bad input results `array`")
		}
		if isGlob := i.File.Glob != "" || strings.Contains(i.File.Path, "*"); !isGlob {
			return fmt.Errorf("file input results defined as array has to be a glob path")
		}
	}
	return nil
}

// Valid is a validation check required for a Benchmark to be considered valid
// and be executed. It checks that all rules and input specs are actually
// valid.
func (b *Benchmark) Valid() error {
	if len(b.Rules) == 0 {
		return fmt.Errorf("bad benchmark: empty rule set")
	}
	for _, rule := range b.Rules {
		if len(rule.InputSpecs) == 0 {
			return fmt.Errorf("missing inputs from rule %s", rule.ID)
		}
		for _, spec := range rule.InputSpecs {
			if err := spec.Valid(); err != nil {
				return fmt.Errorf("bad benchmark: invalid input spec: %w", err)
			}
		}
	}
	return nil
}

// LoadBenchmarks will read the benchmark files that are contained in the
// given root directory, with a name matching the specified glob. If a
// ruleFilter is specified, the loaded benchmarks' rules are filtered. If a
// benchmarks has no rules after the filter is applied, it is not part of the
// results.
func LoadBenchmarks(rootDir, glob string, ruleFilter RuleFilter) ([]*Benchmark, error) {
	filenames := listBenchmarksFilenames(rootDir, glob)
	benchmarks := make([]*Benchmark, 0)
	for _, filename := range filenames {
		b, err := loadFile(rootDir, filename)
		if err != nil {
			return nil, err
		}
		var benchmark Benchmark
		switch filepath.Ext(filename) {
		case ".json":
			err = json.Unmarshal(b, &benchmark)
		default:
			err = yaml.Unmarshal(b, &benchmark)
		}
		if err != nil {
			return nil, err
		}
		benchmark.dirname = rootDir
		if err := benchmark.Valid(); err != nil {
			return nil, err
		}
		var rules []*Rule
		for _, rule := range benchmark.Rules {
			if ruleFilter == nil || ruleFilter(rule) {
				rules = append(rules, rule)
			}
		}
		benchmark.Rules = rules
		if len(rules) > 0 {
			benchmarks = append(benchmarks, &benchmark)
		}
	}
	return benchmarks, nil
}

func listBenchmarksFilenames(rootDir string, glob string) []string {
	if glob == "" {
		return nil
	}
	pattern := filepath.Join(rootDir, filepath.Base(glob))
	paths, _ := filepath.Glob(pattern) // Only possible error is a ErrBadPatter which we ignore.
	for i, path := range paths {
		paths[i] = filepath.Base(path)
	}
	sort.Strings(paths)
	return paths
}

func loadFile(rootDir, filename string) ([]byte, error) {
	path := filepath.Join(rootDir, filepath.Join("/", filename))
	return os.ReadFile(path)
}
