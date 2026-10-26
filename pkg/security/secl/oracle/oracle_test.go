// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package oracle

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/hashicorp/go-multierror"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	update     = flag.Bool("update", false, "rewrite the golden files from the current implementation")
	caseFilter = flag.String("case", "", "only run the cases whose name matches this regexp, golden files are then only checked")
)

// corpusFile is the content of one testdata/corpus/*.yaml file
type corpusFile struct {
	Cases []*corpusCase `yaml:"cases"`
}

type corpusCase struct {
	Name string `yaml:"name"`

	// Macros and Rules make up the main policy. Rules can be given as a bare
	// expression, in which case they are named r0, r1, ...
	Macros []*rules.MacroDefinition `yaml:"macros"`
	Rules  []ruleEntry              `yaml:"rules"`
	// Policies are extra raw policies loaded after the main one, used to
	// exercise macro/rule merging and overriding.
	Policies []string `yaml:"policies"`
	// AgentVersion enables the agent version rule and macro filters
	AgentVersion string `yaml:"agent_version"`
	// RuleCache enables the parsing context rule cache
	RuleCache bool `yaml:"rule_cache"`
	// EventTypesEnabled replaces the default `*` enabled event types
	EventTypesEnabled []string `yaml:"event_types_enabled"`
	// AcceptBPFFilters makes network_filter BPF filters valid, the test
	// build rejecting them all otherwise
	AcceptBPFFilters bool `yaml:"accept_bpf_filters"`
	// RejectMatches makes the first ruleset listener return false for these
	// rules, so that the second listener is not notified of their matches
	RejectMatches []string `yaml:"reject_matches"`
	// Providers are extra policy providers, loaded after the one serving
	// the main policy and Policies
	Providers []providerEntry `yaml:"providers"`

	// Caps are the approver field capabilities per event type. When empty, a
	// capability accepting every value type is generated for every field of
	// the event type used by the rules.
	Caps map[string][]capEntry `yaml:"caps"`
	// Discarders are the fields for which discarders are looked for. When
	// empty, every field used by the rules is a supported discarder.
	Discarders      []string              `yaml:"discarders"`
	MultiDiscarders []multiDiscarderEntry `yaml:"multi_discarders"`

	Processes map[string]*processEntry `yaml:"processes"`
	Events    []*eventEntry            `yaml:"events"`
}

type providerEntry struct {
	// Type is the provider type, e.g. remote-config, file, bundled
	Type     string `yaml:"type"`
	Policies []struct {
		Name         string `yaml:"name"`
		InternalType string `yaml:"internal_type"`
		Content      string `yaml:"content"`
	} `yaml:"policies"`
}

type ruleEntry struct {
	rules.RuleDefinition
}

func (r *ruleEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		r.Expression = node.Value
		return nil
	}
	return node.Decode(&r.RuleDefinition)
}

type capEntry struct {
	Field     string   `yaml:"field"`
	Types     []string `yaml:"types"`
	Weight    int      `yaml:"weight"`
	Mode      string   `yaml:"mode"`
	Range     *[2]int  `yaml:"range"`
	HandleNot bool     `yaml:"handle_not"`
	// Validate rejects the listed filter values
	Reject []string `yaml:"reject"`
}

type multiDiscarderEntry struct {
	Entries []struct {
		Field     string `yaml:"field"`
		EventType string `yaml:"event_type"`
	} `yaml:"entries"`
	FinalField     string `yaml:"final_field"`
	FinalEventType string `yaml:"final_event_type"`
}

type processEntry struct {
	Pid    uint32                 `yaml:"pid"`
	Parent string                 `yaml:"parent"`
	Fields map[string]interface{} `yaml:"fields"`

	pce *model.ProcessCacheEntry
}

type eventEntry struct {
	Type    string                 `yaml:"type"`
	Process string                 `yaml:"process"`
	Fields  map[string]interface{} `yaml:"fields"`
	// Sleep pauses before the event is evaluated, to let variable TTLs expire
	Sleep string `yaml:"sleep"`
	// Cleanup runs CleanupExpiredVariables before the event is evaluated
	Cleanup bool `yaml:"cleanup"`
	// Reparent snapshots the inherited variables of a named process with
	// CopyInheritedVariables, then moves it under another named process (or
	// makes it an orphan when parent is empty), before the event is evaluated
	Reparent *reparentEntry `yaml:"reparent"`
}

type reparentEntry struct {
	Process string `yaml:"process"`
	Parent  string `yaml:"parent"`
}

// observation is what the golden files record for one case
type observation struct {
	LoadPanic  string                          `yaml:"load_panic,omitempty"`
	LoadErrors []string                        `yaml:"load_errors,omitempty"`
	Filtered   []string                        `yaml:"filtered_rules,omitempty"`
	Rules      map[string]*ruleObservation     `yaml:"rules,omitempty"`
	Approvers  map[string]*approverObservation `yaml:"approvers,omitempty"`
	Discarders map[string]string               `yaml:"discarders_report,omitempty"`
	// RuleSet level accessors, which aggregate what is observed per rule
	EventTypes   []string            `yaml:"event_types,omitempty"`
	MacroIDs     []string            `yaml:"macro_ids,omitempty"`
	FieldValues  map[string][]string `yaml:"field_values,omitempty"`
	Policies     []string            `yaml:"policies,omitempty"`
	OnDemandHook []string            `yaml:"on_demand_hook_points,omitempty"`
	RuleIDs      []string            `yaml:"rule_ids,omitempty"`
	// ApproversAll is what RuleSet.GetApprovers reports for all the event
	// types at once, compared to the per event type results
	ApproversAll []string `yaml:"approvers_all,omitempty"`
	// PolicyRules is the merge state of every rule of every policy
	PolicyRules []string           `yaml:"policy_rules,omitempty"`
	Steps       []*stepObservation `yaml:"steps,omitempty"`
}

type ruleObservation struct {
	EventType   string              `yaml:"event_type,omitempty"`
	Fields      []string            `yaml:"fields,omitempty"`
	FieldValues map[string][]string `yaml:"field_values,omitempty"`
}

type approverObservation struct {
	Error      string              `yaml:"error,omitempty"`
	AcceptMode string              `yaml:"accept_mode,omitempty"`
	Approvers  map[string][]string `yaml:"approvers,omitempty"`
	FieldStats map[string]int      `yaml:"field_stats,omitempty"`
	NoDiscard  []string            `yaml:"no_discarder_rules,omitempty"`
}

type stepObservation struct {
	EventError string   `yaml:"event_error,omitempty"`
	Panic      string   `yaml:"panic,omitempty"`
	Result     bool     `yaml:"result"`
	Matched    []string `yaml:"matched,omitempty"`
	// SecondListener are the matches the second listener was notified of
	SecondListener []string                     `yaml:"second_listener,omitempty"`
	Matching       map[string][]string          `yaml:"matching_values,omitempty"`
	Partials       map[string]map[string]string `yaml:"partials,omitempty"`
	Discarders     []string                     `yaml:"discarders,omitempty"`
	// Logs are the ruleset logger lines emitted while evaluating the event
	Logs []string `yaml:"logs,omitempty"`
	// ScopedCounts is, per scoped variable, the number of scopes holding a value
	ScopedCounts map[string]int    `yaml:"scoped_counts,omitempty"`
	Variables    map[string]string `yaml:"variables,omitempty"`
}

// recordingLogger records the ruleset logs, the target being switched per step
type recordingLogger struct {
	lines *[]string
}

func (l *recordingLogger) record(level, format string, params ...interface{}) {
	if l.lines != nil {
		*l.lines = append(*l.lines, level+": "+fmt.Sprintf(format, params...))
	}
}

func (l *recordingLogger) Infof(format string, params ...interface{}) {
	l.record("info", format, params...)
}
func (l *recordingLogger) Tracef(format string, params ...interface{}) {
	l.record("trace", format, params...)
}
func (l *recordingLogger) Warnf(format string, params ...interface{}) {
	l.record("warn", format, params...)
}
func (l *recordingLogger) Debugf(format string, params ...interface{}) {
	l.record("debug", format, params...)
}
func (l *recordingLogger) Errorf(format string, params ...interface{}) {
	l.record("error", format, params...)
}
func (l *recordingLogger) IsTracing() bool { return false }

// listener records what the ruleset notifies for one event
type listener struct {
	step   *stepObservation
	reject []string
}

// secondListener records the matches the first listener let through
type secondListener struct {
	first *listener
}

func (l *secondListener) RuleMatch(_ *eval.Context, rule *rules.Rule, _ eval.Event) bool {
	l.first.step.SecondListener = append(l.first.step.SecondListener, rule.ID)
	return true
}

func (l *secondListener) EventDiscarderFound(*rules.RuleSet, eval.Event, eval.Field, eval.EventType) {
}

func (l *listener) RuleMatch(ctx *eval.Context, rule *rules.Rule, _ eval.Event) bool {
	l.step.Matched = append(l.step.Matched, rule.ID)
	subExprs := ctx.GetMatchingSubExprs()
	for _, pos := range subExprs.GetMatchingValuePos(rule.Expression) {
		if l.step.Matching == nil {
			l.step.Matching = make(map[string][]string)
		}
		l.step.Matching[rule.ID] = append(l.step.Matching[rule.ID], fmt.Sprintf("%s=%s@%d+%d", pos.Field, fmtValue(pos.Value), pos.Offset, pos.Length))
	}
	return !slices.Contains(l.reject, rule.ID)
}

func (l *listener) EventDiscarderFound(_ *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	value, err := event.GetFieldValue(field)
	if err != nil {
		value = "error: " + err.Error()
	}
	l.step.Discarders = append(l.step.Discarders, fmt.Sprintf("%s:%s=%s", eventType, field, fmtValue(value)))
}

// policyProvider serves in-memory policies
type policyProvider struct {
	policies []string
	// entry, when set, replaces policies
	entry *providerEntry
}

func (p *policyProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	var (
		policies []*rules.Policy
		errs     *multierror.Error
	)
	load := func(info *rules.PolicyInfo, content string) {
		policy, err := rules.LoadPolicy(info, strings.NewReader(content), macroFilters, ruleFilters)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		if policy != nil {
			policies = append(policies, policy)
		}
	}
	if p.entry != nil {
		for _, ep := range p.entry.Policies {
			load(&rules.PolicyInfo{Name: ep.Name, Source: p.entry.Type, InternalType: rules.InternalPolicyType(ep.InternalType)}, ep.Content)
		}
		return policies, errs
	}
	for i, content := range p.policies {
		load(&rules.PolicyInfo{Name: fmt.Sprintf("policy%d", i), Source: "oracle"}, content)
	}
	return policies, errs
}

func (p *policyProvider) SetOnNewPoliciesReadyCb(func(bool)) {}
func (p *policyProvider) Start()                             {}
func (p *policyProvider) Close() error                       { return nil }
func (p *policyProvider) Type() string {
	if p.entry != nil {
		return p.entry.Type
	}
	return "oracle"
}

func TestOracle(t *testing.T) {
	files, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("empty corpus")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		t.Run(name, func(t *testing.T) {
			runCorpusFile(t, file, filepath.Join("testdata", "golden", name+".yaml"))
		})
	}
}

// TestOracleOrderIndependence checks that no case depends on the cases run
// before it, as compilation state leaking from one RuleSet to the next would
// make the golden files depend on the order of the cases. Every corpus file is
// replayed in reverse order in a fresh process and compared with a forward
// run of the whole corpus in this process.
func TestOracleOrderIndependence(t *testing.T) {
	files, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil {
		t.Fatal(err)
	}

	forward := make(map[string]map[string]string)
	for _, file := range files {
		forward[file] = make(map[string]string)
		for _, c := range loadCorpus(t, file).Cases {
			forward[file][c.Name] = toYAML(t, runCase(t, c))
		}
	}

	for _, file := range files {
		out := filepath.Join(t.TempDir(), "observations.yaml")
		cmd := exec.Command(os.Args[0], "-test.run", "^TestOracleReverseDump$")
		cmd.Env = append(os.Environ(), "ORACLE_REVERSE_FILE="+file, "ORACLE_REVERSE_OUT="+out)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("reverse run of %s failed: %v\n%s", file, err, output)
		}
		content, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		var backward map[string]string
		if err := yaml.Unmarshal(content, &backward); err != nil {
			t.Fatal(err)
		}
		for name, observed := range forward[file] {
			if backward[name] != observed {
				t.Errorf("case `%s` of %s depends on the cases run before it\n--- forward\n%s\n--- reverse, alone\n%s", name, file, observed, backward[name])
			}
		}
	}
}

// TestOracleReverseDump is the helper process of TestOracleOrderIndependence
func TestOracleReverseDump(t *testing.T) {
	file, out := os.Getenv("ORACLE_REVERSE_FILE"), os.Getenv("ORACLE_REVERSE_OUT")
	if file == "" || out == "" {
		t.Skip("helper process of TestOracleOrderIndependence")
	}

	cases := loadCorpus(t, file).Cases
	observations := make(map[string]string)
	for i := len(cases) - 1; i >= 0; i-- {
		observations[cases[i].Name] = toYAML(t, runCase(t, cases[i]))
	}
	if err := os.WriteFile(out, []byte(toYAML(t, observations)), 0644); err != nil {
		t.Fatal(err)
	}
}

func loadCorpus(t *testing.T, corpusPath string) *corpusFile {
	content, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}

	var corpus corpusFile
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("invalid corpus %s: %v", corpusPath, err)
	}
	return &corpus
}

func runCorpusFile(t *testing.T, corpusPath, goldenPath string) {
	corpus := loadCorpus(t, corpusPath)

	names := make(map[string]bool)
	observations := make(map[string]*observation)
	var filterRe *regexp.Regexp
	if *caseFilter != "" {
		if *update {
			t.Fatal("-case can't be used with -update")
		}
		filterRe = regexp.MustCompile(*caseFilter)
	}

	var cases []*corpusCase
	for _, c := range corpus.Cases {
		if c.Name == "" || names[c.Name] {
			t.Fatalf("case names must be unique and non empty: `%s`", c.Name)
		}
		names[c.Name] = true
		if filterRe == nil || filterRe.MatchString(c.Name) {
			cases = append(cases, c)
			observations[c.Name] = runCase(t, c)
		}
	}

	if *update {
		out, err := yaml.Marshal(observations)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, out, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	goldenContent, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden file, run with -update: %v", err)
	}
	var golden map[string]interface{}
	if err := yaml.Unmarshal(goldenContent, &golden); err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			expected, ok := golden[c.Name]
			if !ok {
				t.Fatal("case missing from the golden file, run with -update")
			}
			actual := normalize(t, observations[c.Name])
			if !reflect.DeepEqual(normalize(t, expected), actual) {
				t.Errorf("observation mismatch\n--- expected\n%s\n--- actual\n%s", toYAML(t, expected), toYAML(t, actual))
			}
		})
	}
	for name := range golden {
		if filterRe == nil && !names[name] {
			t.Errorf("golden case `%s` has no corpus case, run with -update", name)
		}
	}
}

func normalize(t *testing.T, v interface{}) interface{} {
	var out interface{}
	if err := yaml.Unmarshal([]byte(toYAML(t, v)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func toYAML(t *testing.T, v interface{}) string {
	out, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func (c *corpusCase) mainPolicy() (string, error) {
	def := rules.PolicyDef{Macros: c.Macros}
	for i := range c.Rules {
		rule := c.Rules[i].RuleDefinition
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("r%d", i)
		}
		def.Rules = append(def.Rules, &rule)
	}
	out, err := yaml.Marshal(def)
	return string(out), err
}

func errorStrings(err error) []string {
	var merr *multierror.Error
	if errors.As(err, &merr) {
		var out []string
		for _, e := range merr.Errors {
			out = append(out, errorStrings(e)...)
		}
		return out
	}
	// some error types panic when formatted (typed nil, missing fields)
	var msg string
	func() {
		defer catch(&msg)
		msg = err.Error()
	}()
	return []string{msg}
}

func runCase(t *testing.T, c *corpusCase) *observation {
	obs := &observation{}

	enabled := map[eval.EventType]bool{"*": true}
	if len(c.EventTypesEnabled) > 0 {
		enabled = make(map[eval.EventType]bool)
		for _, eventType := range c.EventTypesEnabled {
			enabled[eventType] = true
		}
	}
	ruleOpts, evalOpts := rules.NewBothOpts(enabled)
	ruleOpts.WithRuleCacheEnabled(c.RuleCache)
	logger := &recordingLogger{}
	ruleOpts.WithLogger(logger)
	rs := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)

	mainPolicy, err := c.mainPolicy()
	if err != nil {
		t.Fatal(err)
	}
	providers := []rules.PolicyProvider{&policyProvider{policies: append([]string{mainPolicy}, c.Policies...)}}
	for i := range c.Providers {
		providers = append(providers, &policyProvider{entry: &c.Providers[i]})
	}
	loader := rules.NewPolicyLoader(providers...)
	var loaderOpts rules.PolicyLoaderOpts
	if c.AcceptBPFFilters {
		loaderOpts.ValidateBPFFilter = func(string) error { return nil }
	}
	if c.AgentVersion != "" {
		version, err := semver.NewVersion(c.AgentVersion)
		if err != nil {
			t.Fatal(err)
		}
		filter, err := rules.NewAgentVersionFilter(version)
		if err != nil {
			t.Fatal(err)
		}
		loaderOpts.RuleFilters = []rules.RuleFilter{filter}
		loaderOpts.MacroFilters = []rules.MacroFilter{filter}
	}
	var (
		filtered []*rules.PolicyRule
		errs     *multierror.Error
	)
	func() {
		defer catch(&obs.LoadPanic)
		filtered, errs = rs.LoadPolicies(loader, loaderOpts)
	}()
	if obs.LoadPanic != "" {
		return obs
	}
	for _, rule := range filtered {
		obs.Filtered = append(obs.Filtered, rule.Def.ID)
	}
	if errs.ErrorOrNil() != nil {
		obs.LoadErrors = errorStrings(errs)
		sort.Strings(obs.LoadErrors)
	}

	// rules expanded from a single definition share its ID, sort them by expression too
	allRules := rs.GetRules()
	sort.SliceStable(allRules, func(i, j int) bool {
		if allRules[i].ID != allRules[j].ID {
			return allRules[i].ID < allRules[j].ID
		}
		return allRules[i].Expression < allRules[j].Expression
	})

	// supported discarders have to be set after the load as the opts are only read at evaluation time
	discarders := c.Discarders
	if len(discarders) == 0 {
		discarders = rs.ListFields()
	}
	supported := make(map[eval.Field]bool)
	for _, field := range discarders {
		supported[field] = true
	}
	ruleOpts.WithSupportedDiscarders(supported)
	var mdiscs []*rules.MultiDiscarder
	for _, md := range c.MultiDiscarders {
		mdisc := &rules.MultiDiscarder{FinalField: md.FinalField, FinalEventType: parseEventType(t, md.FinalEventType)}
		for _, e := range md.Entries {
			mdisc.Entries = append(mdisc.Entries, rules.MultiDiscarderEntry{Field: e.Field, EventType: parseEventType(t, e.EventType)})
		}
		mdiscs = append(mdiscs, mdisc)
	}
	ruleOpts.WithSupportedMultiDiscarder(mdiscs)

	obs.Rules = observeRules(allRules)
	observeRuleSet(t, rs, c, obs)
	var noDiscarderRules map[eval.RuleID]bool
	obs.Approvers, noDiscarderRules = observeApprovers(t, rs, c, obs)
	// as done by kfilters, rules using approver only filters can't generate discarders
	rs.WithExcludedRuleFromDiscarders(noDiscarderRules)
	obs.Discarders = observeDiscardersReport(rs, ruleOpts, discarders)
	ruleOpts.WithSupportedDiscarders(supported)

	l := &listener{reject: c.RejectMatches}
	rs.AddListener(l)
	rs.AddListener(&secondListener{first: l})

	buildProcesses(t, c)

	for i, e := range c.Events {
		step := &stepObservation{}
		l.step = step
		logger.lines = &step.Logs
		obs.Steps = append(obs.Steps, step)

		if e.Sleep != "" {
			d, err := time.ParseDuration(e.Sleep)
			if err != nil {
				t.Fatalf("event %d: %v", i, err)
			}
			time.Sleep(d)
		}
		if e.Cleanup {
			rs.CleanupExpiredVariables()
		}
		if e.Reparent != nil {
			reparent(t, rs, c, e.Reparent)
		}

		event, err := buildEvent(t, c, e)
		if err != nil {
			step.EventError = err.Error()
			continue
		}

		func() {
			defer catch(&step.Panic)
			step.Result = rs.Evaluate(event)
			if !step.Result {
				rs.EvaluateDiscarders(event)
			}
		}()
		sort.Strings(step.Discarders)

		for _, rule := range allRules {
			if rule.GetEvaluator() == nil {
				continue
			}
			if ruleType, _ := rule.GetEventType(); ruleType != event.GetType() {
				continue
			}
			for _, field := range rule.GetFields() {
				var result string
				func() {
					defer catch(&result)
					// a context per partial evaluation, as registers and caches
					// would otherwise leak between rules
					ok, err := rule.PartialEval(eval.NewContext(event), field)
					if err != nil {
						result = "error: " + err.Error()
					} else {
						result = fmt.Sprint(ok)
					}
				}()
				if step.Partials == nil {
					step.Partials = make(map[string]map[string]string)
				}
				if step.Partials[rule.ID] == nil {
					step.Partials[rule.ID] = make(map[string]string)
				}
				step.Partials[rule.ID][field] = result
			}
		}

		func() {
			var panicked string
			defer func() {
				if panicked != "" {
					step.Variables = map[string]string{"panic": panicked}
				}
			}()
			defer catch(&panicked)
			step.Variables = observeVariables(rs, eval.NewContext(event))
		}()
		step.ScopedCounts = observeScopedCounts(rs)
		logger.lines = nil
	}

	return obs
}

// observeRuleSet records the ruleset wide accessors
func observeRuleSet(t *testing.T, rs *rules.RuleSet, c *corpusCase, obs *observation) {
	for _, eventType := range rs.GetEventTypes() {
		if !rs.HasRulesForEventType(eventType) {
			t.Fatalf("event type `%s` listed without rules", eventType)
		}
		obs.EventTypes = append(obs.EventTypes, eventType)
	}
	sort.Strings(obs.EventTypes)
	for _, e := range c.Events {
		if !slices.Contains(obs.EventTypes, e.Type) && rs.HasRulesForEventType(e.Type) {
			obs.EventTypes = append(obs.EventTypes, "unlisted:"+e.Type)
		}
	}

	for _, id := range rs.ListMacroIDs() {
		obs.MacroIDs = append(obs.MacroIDs, id)
	}
	sort.Strings(obs.MacroIDs)

	for _, field := range rs.ListFields() {
		var values []string
		for _, fv := range rs.GetFieldValues(field) {
			values = append(values, fmt.Sprintf("%s:%s", fv.Type, fmtValue(fv.Value)))
		}
		sort.Strings(values)
		if len(values) > 0 {
			if obs.FieldValues == nil {
				obs.FieldValues = make(map[string][]string)
			}
			obs.FieldValues[field] = values
		}
	}

	for _, policy := range rs.GetPolicies() {
		var ids []string
		for _, rule := range policy.GetAcceptedRules() {
			ids = append(ids, rule.Def.ID)
		}
		sort.Strings(ids)
		info := policy.Info
		obs.Policies = append(obs.Policies, fmt.Sprintf("%s(%s) internal_type=%s type=%s version=%s replace=%s %v",
			info.Name, info.Source, info.InternalType, info.Type, info.Version, info.ReplacePolicyID, ids))
		for _, rule := range policy.Rules {
			obs.PolicyRules = append(obs.PolicyRules, fmtPolicyRule(info.Name, rule))
		}
	}

	for _, rule := range rs.GetRules() {
		if byID := rs.GetRuleByID(rule.ID); byID == nil || rs.GetRuleMap()[rule.ID] != byID {
			obs.Policies = append(obs.Policies, "inconsistent rule lookup: "+rule.ID)
		}
	}

	for _, id := range rs.ListRuleIDs() {
		obs.RuleIDs = append(obs.RuleIDs, id)
	}
	sort.Strings(obs.RuleIDs)

	// hook points and their args come out of maps
	hookPoints, err := rs.GetOnDemandHookPoints()
	if err != nil {
		obs.OnDemandHook = []string{"error: " + err.Error()}
	}
	for _, hp := range hookPoints {
		args := slices.Clone(hp.Args)
		sort.Slice(args, func(i, j int) bool { return args[i].N < args[j].N })
		obs.OnDemandHook = append(obs.OnDemandHook, fmt.Sprintf("%s syscall=%v args=%v", hp.Name, hp.IsSyscall, args))
	}
	sort.Strings(obs.OnDemandHook)
}

func fmtPolicyRule(policyName string, rule *rules.PolicyRule) string {
	names := func(infos []rules.PolicyInfo) []string {
		var out []string
		for _, info := range infos {
			out = append(out, info.Name)
		}
		return out
	}
	def := rule.Def
	var every string
	if def.Every != nil {
		every = def.Every.String()
	}
	var tags []string
	for _, k := range sortedKeys(def.Tags) {
		tags = append(tags, k+"="+def.Tags[k])
	}
	return fmt.Sprintf("%s/%s accepted=%v disabled=%v expression=%q tags=%v product_tags=%v every=%s actions=%d policy=%s modified_by=%v used_by=%v enable_count=%d",
		policyName, def.ID, rule.Accepted, def.Disabled, def.Expression, tags, def.ProductTags, every, len(def.Actions),
		rule.Policy.Name, names(rule.ModifiedBy), names(rule.UsedBy), rule.EnableCount)
}

func reparent(t *testing.T, rs *rules.RuleSet, c *corpusCase, r *reparentEntry) {
	p := c.Processes[r.Process]
	if p == nil {
		t.Fatalf("unknown process `%s`", r.Process)
	}
	rs.CopyInheritedVariables(p.pce)
	if r.Parent == "" {
		p.pce.Ancestor, p.pce.Parent, p.pce.PPid = nil, nil, 0
		return
	}
	parent := c.Processes[r.Parent]
	if parent == nil {
		t.Fatalf("unknown process `%s`", r.Parent)
	}
	p.pce.Ancestor, p.pce.Parent, p.pce.PPid = parent.pce, &parent.pce.Process, parent.pce.Pid
}

// observeDiscardersReport reports field by field so that a field that can't be
// checked doesn't hide the others
func observeDiscardersReport(rs *rules.RuleSet, ruleOpts *rules.Opts, fields []string) map[string]string {
	out := make(map[string]string)
	for _, field := range fields {
		ruleOpts.WithSupportedDiscarders(map[eval.Field]bool{field: true})
		out[field] = func() (result string) {
			defer func() {
				if r := recover(); r != nil {
					result = fmt.Sprintf("panic: %v", r)
				}
			}()
			report, err := rs.GetDiscardersReport()
			switch {
			case err != nil:
				return "error: " + err.Error()
			case len(report.Supported) > 0:
				return "supported"
			case len(report.Invalid) > 0:
				return "invalid: " + report.Invalid[0].RuleID
			default:
				return "none"
			}
		}()
	}
	return out
}

func parseEventType(t *testing.T, name string) model.EventType {
	for i := model.UnknownEventType; i < model.MaxAllEventType; i++ {
		if i.String() == name {
			return i
		}
	}
	t.Fatalf("unknown event type `%s`", name)
	return model.UnknownEventType
}

func observeRules(allRules []*rules.Rule) map[string]*ruleObservation {
	out := make(map[string]*ruleObservation)
	for _, rule := range allRules {
		ro := &ruleObservation{}
		out[rule.ID] = ro
		if rule.GetEvaluator() == nil {
			continue
		}
		eventType, err := rule.GetEventType()
		if err != nil {
			ro.EventType = "error: " + err.Error()
		} else {
			ro.EventType = eventType
		}
		ro.Fields = slices.Clone(rule.GetFields())
		sort.Strings(ro.Fields)
		for _, field := range ro.Fields {
			for _, fv := range rule.GetFieldValues(field) {
				if ro.FieldValues == nil {
					ro.FieldValues = make(map[string][]string)
				}
				ro.FieldValues[field] = append(ro.FieldValues[field], fmt.Sprintf("%s:%s", fv.Type, fmtValue(fv.Value)))
			}
		}
	}
	return out
}

var valueTypes = map[string]eval.FieldValueType{
	"scalar":  eval.ScalarValueType,
	"glob":    eval.GlobValueType,
	"pattern": eval.PatternValueType,
	"regexp":  eval.RegexpValueType,
	"bitmask": eval.BitmaskValueType,
	"range":   eval.RangeValueType,
}

func (ce *capEntry) toCapability(t *testing.T) *rules.FieldCapability {
	fc := &rules.FieldCapability{
		Field:        ce.Field,
		FilterWeight: ce.Weight,
	}
	for _, name := range ce.Types {
		vt, ok := valueTypes[name]
		if !ok {
			t.Fatalf("unknown value type `%s`", name)
		}
		fc.TypeBitmask |= vt
	}
	switch ce.Mode {
	case "", "default":
	case "approver_only":
		fc.FilterMode = rules.ApproverOnlyMode
	default:
		t.Fatalf("unknown filter mode `%s`", ce.Mode)
	}
	if ce.Range != nil {
		fc.RangeFilterValue = &rules.RangeFilterValue{Min: ce.Range[0], Max: ce.Range[1]}
	}
	if ce.HandleNot {
		// mirrors the kfilters handling of `!=` on a flag-like field: the not
		// value is turned into a bitmask approver covering every other bit
		fc.HandleNotApproverValue = func(fieldValueType eval.FieldValueType, value interface{}) (eval.FieldValueType, interface{}, bool) {
			if fieldValueType != eval.ScalarValueType {
				return fieldValueType, value, false
			}
			i, ok := value.(int)
			if !ok {
				return fieldValueType, value, false
			}
			return eval.BitmaskValueType, ^i, true
		}
	}
	if len(ce.Reject) > 0 {
		reject := ce.Reject
		fc.ValidateFnc = func(fv rules.FilterValue) bool {
			return !slices.Contains(reject, fmtValue(fv.Value))
		}
	}
	return fc
}

func observeApprovers(t *testing.T, rs *rules.RuleSet, c *corpusCase, obs *observation) (map[string]*approverObservation, map[eval.RuleID]bool) {
	out := make(map[string]*approverObservation)
	noDiscarders := make(map[eval.RuleID]bool)

	eventTypes := rs.GetEventTypes()
	sort.Strings(eventTypes)
	allCaps := make(map[eval.EventType]rules.FieldCapabilities)
	perType := make(map[eval.EventType]rules.Approvers)

	for _, eventType := range eventTypes {
		var caps rules.FieldCapabilities
		if entries, ok := c.Caps[eventType]; ok {
			for i := range entries {
				caps = append(caps, entries[i].toCapability(t))
			}
		} else if len(c.Caps) == 0 {
			fieldSet := make(map[string]bool)
			for _, rule := range rs.GetBucket(eventType).GetRules() {
				for _, field := range rule.GetFields() {
					fieldSet[field] = true
				}
			}
			for _, field := range sortedKeys(fieldSet) {
				if !strings.HasPrefix(field, eventType+".") {
					continue
				}
				caps = append(caps, &rules.FieldCapability{
					Field:       field,
					TypeBitmask: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType | eval.BitmaskValueType | eval.RangeValueType,
				})
			}
		}

		allCaps[eventType] = caps
		ao := &approverObservation{}
		out[eventType] = ao

		var (
			approvers        rules.Approvers
			stats            *rules.ApproverStats
			noDiscarderRules []*rules.Rule
			err              error
		)
		func() {
			defer catch(&ao.Error)
			approvers, stats, noDiscarderRules, err = rs.GetEventTypeApprovers(eventType, caps)
		}()
		perType[eventType] = approvers
		if err != nil {
			ao.Error = err.Error()
		}
		if stats != nil {
			if rule := stats.AcceptModeRules[eventType]; rule != nil {
				ao.AcceptMode = rule.ID
			}
			if len(stats.FieldStats) > 0 {
				ao.FieldStats = stats.FieldStats
			}
		}
		for field, values := range approvers {
			if ao.Approvers == nil {
				ao.Approvers = make(map[string][]string)
			}
			for _, fv := range values {
				mode := ""
				if fv.Mode == rules.ApproverOnlyMode {
					mode = ":approver_only"
				}
				ao.Approvers[field] = append(ao.Approvers[field], fmt.Sprintf("%s:%s%s", fv.Type, fmtValue(fv.Value), mode))
			}
		}
		for _, rule := range noDiscarderRules {
			ao.NoDiscard = append(ao.NoDiscard, rule.ID)
			noDiscarders[rule.ID] = true
		}
	}

	observeAllApprovers(rs, allCaps, perType, obs)
	return out, noDiscarders
}

func observeAllApprovers(rs *rules.RuleSet, caps map[eval.EventType]rules.FieldCapabilities, perType map[eval.EventType]rules.Approvers, obs *observation) {
	var (
		approvers        map[eval.EventType]rules.Approvers
		stats            *rules.ApproverStats
		noDiscarderRules []*rules.Rule
		err              error
		panicked         string
	)
	func() {
		defer catch(&panicked)
		approvers, stats, noDiscarderRules, err = rs.GetApprovers(caps)
	}()
	switch {
	case panicked != "":
		obs.ApproversAll = []string{panicked}
		return
	case err != nil:
		obs.ApproversAll = []string{"error: " + err.Error()}
		return
	}
	union := make(map[eval.EventType]bool)
	for eventType := range approvers {
		union[eventType] = true
	}
	for eventType := range perType {
		union[eventType] = true
	}
	for _, eventType := range sortedKeys(union) {
		a, b := approvers[eventType], perType[eventType]
		if (len(a) != 0 || len(b) != 0) && !reflect.DeepEqual(a, b) {
			obs.ApproversAll = append(obs.ApproversAll, "differs from per event type: "+eventType)
		}
	}
	if stats != nil {
		for _, eventType := range sortedKeys(stats.AcceptModeRules) {
			obs.ApproversAll = append(obs.ApproversAll, fmt.Sprintf("accept_mode %s=%s", eventType, stats.AcceptModeRules[eventType].ID))
		}
		for _, field := range sortedKeys(stats.FieldStats) {
			obs.ApproversAll = append(obs.ApproversAll, fmt.Sprintf("field_stats %s=%d", field, stats.FieldStats[field]))
		}
	}
	var ids []string
	for _, rule := range noDiscarderRules {
		ids = append(ids, "no_discarder "+rule.ID)
	}
	sort.Strings(ids)
	obs.ApproversAll = append(obs.ApproversAll, ids...)
}

func observeVariables(rs *rules.RuleSet, ctx *eval.Context) map[string]string {
	out := make(map[string]string)
	for name, variable := range rs.GetVariables() {
		if strings.HasPrefix(name, "builtins.") {
			continue
		}
		switch v := variable.(type) {
		case eval.ScopedVariable:
			value, ok := v.GetValue(ctx, false)
			own, ownOk := v.GetValue(ctx, true)
			value, own = sortedArray(value), sortedArray(own)
			out[name] = fmt.Sprintf("%s(%v) own=%s(%v)", fmtValue(value), ok, fmtValue(own), ownOk)
		case eval.Variable:
			value, ok := v.GetValue()
			value = sortedArray(value)
			out[name] = fmt.Sprintf("%s(%v)", fmtValue(value), ok)
		default:
			out[name] = fmt.Sprintf("unknown variable type %T", variable)
		}
	}
	return out
}

func observeScopedCounts(rs *rules.RuleSet) map[string]int {
	var out map[string]int
	for name := range rs.GetVariables() {
		for _, scope := range rules.VariableScopes {
			if !strings.HasPrefix(name, scope+".") {
				continue
			}
			if out == nil {
				out = make(map[string]int)
			}
			out[name] = len(rs.GetScopedVariables(rules.Scope(scope), strings.TrimPrefix(name, scope+".")))
		}
	}
	return out
}

func buildProcesses(t *testing.T, c *corpusCase) {
	for _, p := range c.Processes {
		p.pce = nil
	}

	var build func(name string, depth int) *model.ProcessCacheEntry
	build = func(name string, depth int) *model.ProcessCacheEntry {
		p := c.Processes[name]
		if p == nil {
			t.Fatalf("unknown process `%s`", name)
		}
		if p.pce != nil {
			return p.pce
		}
		if depth > len(c.Processes) {
			t.Fatalf("process loop on `%s`", name)
		}

		ev := model.NewFakeEvent()
		for _, field := range sortedKeys(p.Fields) {
			if err := setField(ev, "process."+field, p.Fields[field]); err != nil {
				t.Fatalf("process `%s`: %v", name, err)
			}
		}
		pce := &model.ProcessCacheEntry{ProcessContext: *ev.ProcessContext}
		pce.Pid = p.Pid
		if p.Pid != 0 && pce.Tid == 0 {
			pce.Tid = p.Pid
		}
		if p.Parent != "" {
			parent := build(p.Parent, depth+1)
			pce.Ancestor = parent
			pce.Parent = &parent.Process
			pce.PPid = parent.Pid
		}
		p.pce = pce
		return pce
	}

	for _, name := range sortedKeys(c.Processes) {
		build(name, 0)
	}
}

func buildEvent(t *testing.T, c *corpusCase, e *eventEntry) (*model.Event, error) {
	ev := model.NewFakeEvent()
	eventType := parseEventType(t, e.Type)
	ev.Type = uint32(eventType)

	if e.Process != "" {
		p := c.Processes[e.Process]
		if p == nil {
			t.Fatalf("unknown process `%s`", e.Process)
		}
		ev.ProcessCacheEntry = p.pce
		ev.ProcessContext = &p.pce.ProcessContext
		handlers := ev.FieldHandlers.(*model.FakeFieldHandlers)
		for _, other := range c.Processes {
			handlers.PCEs[other.pce.Pid] = other.pce
		}
	}

	for _, field := range sortedKeys(e.Fields) {
		if err := setField(ev, field, e.Fields[field]); err != nil {
			return nil, err
		}
	}
	return ev, nil
}

// setField converts the YAML value to the type expected by the field
func setField(ev *model.Event, field string, value interface{}) error {
	_, kind, basicType, isArray, err := ev.GetFieldMetadata(field)
	if err != nil {
		return err
	}

	convert := func(v interface{}) (interface{}, error) {
		switch {
		case basicType == "net.IPNet":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("field `%s` expects an IP or CIDR string", field)
			}
			if !strings.Contains(s, "/") {
				if strings.Contains(s, ":") {
					s += "/128"
				} else {
					s += "/32"
				}
			}
			_, ipnet, err := net.ParseCIDR(s)
			if err != nil {
				return nil, err
			}
			return *ipnet, nil
		case kind == reflect.Int:
			// `now-<duration>` gives a timestamp relative to the evaluation time
			if s, ok := v.(string); ok && strings.HasPrefix(s, "now-") {
				d, err := time.ParseDuration(strings.TrimPrefix(s, "now-"))
				if err != nil {
					return nil, err
				}
				return int(time.Now().Add(-d).UnixNano()), nil
			}
		}
		return v, nil
	}

	if isArray {
		list, ok := value.([]interface{})
		if !ok {
			list = []interface{}{value}
		}
		switch kind {
		case reflect.String:
			var out []string
			for _, v := range list {
				out = append(out, fmt.Sprint(v))
			}
			value = out
		case reflect.Int:
			var out []int
			for _, v := range list {
				cv, err := convert(v)
				if err != nil {
					return err
				}
				out = append(out, cv.(int))
			}
			value = out
		case reflect.Bool:
			var out []bool
			for _, v := range list {
				out = append(out, v.(bool))
			}
			value = out
		}
		if err := ev.SetFieldValue(field, value); err == nil {
			return nil
		}
		// some array fields are only settable one element at a time
		for _, v := range list {
			cv, err := convert(v)
			if err != nil {
				return err
			}
			if err := ev.SetFieldValue(field, cv); err != nil {
				return err
			}
		}
		return nil
	}

	cv, err := convert(value)
	if err != nil {
		return err
	}
	return ev.SetFieldValue(field, cv)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fmtValue(v interface{}) string {
	switch v := v.(type) {
	case nil:
		return "<nil>"
	case string:
		return fmt.Sprintf("%q", v)
	case net.IPNet:
		return v.String()
	case *net.IPNet:
		return v.String()
	case []net.IPNet:
		var out []string
		for _, n := range v {
			out = append(out, n.String())
		}
		return "[" + strings.Join(out, " ") + "]"
	case []*net.IPNet:
		var out []string
		for _, n := range v {
			out = append(out, n.String())
		}
		return "[" + strings.Join(out, " ") + "]"
	case []string:
		var out []string
		for _, s := range v {
			out = append(out, fmt.Sprintf("%q", s))
		}
		return "[" + strings.Join(out, " ") + "]"
	case int:
		// timestamps built from `now-<duration>` are printed relative to now,
		// rounded to the minute
		if now := int(time.Now().UnixNano()); v > now-int(1000*time.Hour) && v < now+int(time.Hour) {
			return fmt.Sprintf("now-%dm", (now-v+int(30*time.Second))/int(time.Minute))
		}
		return fmt.Sprint(v)
	case rules.RangeFilterValue:
		return fmt.Sprintf("range(%d,%d)", v.Min, v.Max)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortedArray returns a sorted copy of an array value: array variables are
// backed by an LRU whose key order isn't deterministic
func sortedArray(v interface{}) interface{} {
	switch v := v.(type) {
	case []string:
		out := slices.Clone(v)
		slices.Sort(out)
		return out
	case []int:
		out := slices.Clone(v)
		slices.Sort(out)
		return out
	case []net.IPNet:
		out := slices.Clone(v)
		slices.SortFunc(out, func(a, b net.IPNet) int { return strings.Compare(a.String(), b.String()) })
		return out
	}
	return v
}

// catch turns a panic into a recorded result so that a case keeps going
func catch(result *string) {
	if r := recover(); r != nil {
		*result = fmt.Sprintf("panic: %v", r)
	}
}
