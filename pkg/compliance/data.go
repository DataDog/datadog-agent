// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
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
	RegoEvaluator  Evaluator = "rego"
	XCCDFEvaluator Evaluator = "xccdf"
)

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
	Tags         []string               `json:"tags"`
	Data         map[string]interface{} `json:"data"`

	errReason error `json:"-"`
}

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

type RuleScope string

const (
	Unscoped               RuleScope = "none"
	DockerScope            RuleScope = "docker"
	KubernetesNodeScope    RuleScope = "kubernetesNode"
	KubernetesClusterScope RuleScope = "kubernetesCluster"
)

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
		XCCDF         *InputSpecXCCDF         `yaml:"xccdf,omitempty" json:"xccdf,omitempty"`
		Constants     *InputSpecConstants     `yaml:"constants,omitempty" json:"constants,omitempty"`

		TagName string `yaml:"tag,omitempty" json:"tag,omitempty"`
		Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	}

	InputSpecFile struct {
		Path   string `yaml:"path" json:"path"`
		Glob   string `yaml:"glob" json:"glob"`
		Parser string `yaml:"parser,omitempty" json:"parser,omitempty"`
	}

	InputSpecProcess struct {
		Name string   `yaml:"name" json:"name"`
		Envs []string `yaml:"envs,omitempty" json:"envs,omitempty"`
	}

	InputSpecGroup struct {
		Name string `yaml:"name" json:"name"`
	}

	InputSpecAudit struct {
		Path string `yaml:"path" json:"path"`
	}

	InputSpecDocker struct {
		Kind string `yaml:"kind" json:"kind"`
	}

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

	InputSpecXCCDF struct {
		Name    string   `yaml:"name" json:"name"`
		Profile string   `yaml:"profile" json:"profile"`
		Rule    string   `yaml:"rule" json:"rule"`
		Rules   []string `yaml:"rules,omitempty" json:"rules,omitempty"`
	}

	InputSpecConstants map[string]interface{}
)

// ResolvedInputs is the generic data structure that is returned by a Resolver.
type ResolvedInputs map[string]interface{}

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

func (r *Rule) IsRego() bool {
	return !r.IsXCCDF()
}

func (r *Rule) IsXCCDF() bool {
	return len(r.InputSpecs) == 1 && r.InputSpecs[0].XCCDF != nil
}

func (r *Rule) HasScope(scope RuleScope) bool {
	for _, s := range r.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

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
