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
	"path/filepath"
	"reflect"
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

var update = flag.Bool("update", false, "rewrite the golden files from the current implementation")

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
}

// observation is what the golden files record for one case
type observation struct {
	LoadErrors []string                        `yaml:"load_errors,omitempty"`
	Filtered   []string                        `yaml:"filtered_rules,omitempty"`
	Rules      map[string]*ruleObservation     `yaml:"rules,omitempty"`
	Approvers  map[string]*approverObservation `yaml:"approvers,omitempty"`
	Discarders map[string]string               `yaml:"discarders_report,omitempty"`
	Steps      []*stepObservation              `yaml:"steps,omitempty"`
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
	EventError string                       `yaml:"event_error,omitempty"`
	Result     bool                         `yaml:"result"`
	Matched    []string                     `yaml:"matched,omitempty"`
	Matching   map[string][]string          `yaml:"matching_values,omitempty"`
	Partials   map[string]map[string]string `yaml:"partials,omitempty"`
	Discarders []string                     `yaml:"discarders,omitempty"`
	Variables  map[string]string            `yaml:"variables,omitempty"`
}

// listener records what the ruleset notifies for one event
type listener struct {
	step *stepObservation
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
	return true
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
}

func (p *policyProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	var (
		policies []*rules.Policy
		errs     *multierror.Error
	)
	for i, content := range p.policies {
		info := &rules.PolicyInfo{Name: fmt.Sprintf("policy%d", i), Source: "oracle"}
		policy, err := rules.LoadPolicy(info, strings.NewReader(content), macroFilters, ruleFilters)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		if policy != nil {
			policies = append(policies, policy)
		}
	}
	return policies, errs
}

func (p *policyProvider) SetOnNewPoliciesReadyCb(func(bool)) {}
func (p *policyProvider) Start()                             {}
func (p *policyProvider) Close() error                       { return nil }
func (p *policyProvider) Type() string                       { return "oracle" }

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

func runCorpusFile(t *testing.T, corpusPath, goldenPath string) {
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

	names := make(map[string]bool)
	observations := make(map[string]*observation)
	for _, c := range corpus.Cases {
		if c.Name == "" || names[c.Name] {
			t.Fatalf("case names must be unique and non empty: `%s`", c.Name)
		}
		names[c.Name] = true
		observations[c.Name] = runCase(t, c)
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

	for _, c := range corpus.Cases {
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
		if !names[name] {
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
	return []string{err.Error()}
}

func runCase(t *testing.T, c *corpusCase) *observation {
	obs := &observation{}

	ruleOpts, evalOpts := rules.NewBothOpts(map[eval.EventType]bool{"*": true})
	rs := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)

	mainPolicy, err := c.mainPolicy()
	if err != nil {
		t.Fatal(err)
	}
	loader := rules.NewPolicyLoader(&policyProvider{policies: append([]string{mainPolicy}, c.Policies...)})
	var loaderOpts rules.PolicyLoaderOpts
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
	filtered, errs := rs.LoadPolicies(loader, loaderOpts)
	for _, rule := range filtered {
		obs.Filtered = append(obs.Filtered, rule.Def.ID)
	}
	if errs.ErrorOrNil() != nil {
		obs.LoadErrors = errorStrings(errs)
		sort.Strings(obs.LoadErrors)
	}

	var allRules []*rules.Rule
	for _, id := range sortedRuleIDs(rs) {
		allRules = append(allRules, rs.GetRuleByID(id))
	}

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
	obs.Approvers = observeApprovers(t, rs, c)
	obs.Discarders = observeDiscardersReport(rs, ruleOpts, discarders)
	ruleOpts.WithSupportedDiscarders(supported)

	l := &listener{}
	rs.AddListener(l)

	buildProcesses(t, c)

	for i, e := range c.Events {
		step := &stepObservation{}
		l.step = step
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

		event, err := buildEvent(t, c, e)
		if err != nil {
			step.EventError = err.Error()
			continue
		}

		step.Result = rs.Evaluate(event)
		if !step.Result {
			rs.EvaluateDiscarders(event)
		}
		sort.Strings(step.Discarders)

		ctx := eval.NewContext(event)
		for _, rule := range allRules {
			if rule.GetEvaluator() == nil {
				continue
			}
			if ruleType, _ := rule.GetEventType(); ruleType != event.GetType() {
				continue
			}
			for _, field := range rule.GetFields() {
				var result string
				ok, err := rule.PartialEval(ctx, field)
				if err != nil {
					result = "error: " + err.Error()
				} else {
					result = fmt.Sprint(ok)
				}
				if step.Partials == nil {
					step.Partials = make(map[string]map[string]string)
				}
				if step.Partials[rule.ID] == nil {
					step.Partials[rule.ID] = make(map[string]string)
				}
				step.Partials[rule.ID][field] = result
			}
		}

		step.Variables = observeVariables(rs, ctx)
	}

	return obs
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

func sortedRuleIDs(rs *rules.RuleSet) []string {
	ids := rs.ListRuleIDs()
	sort.Strings(ids)
	return ids
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

func observeApprovers(t *testing.T, rs *rules.RuleSet, c *corpusCase) map[string]*approverObservation {
	out := make(map[string]*approverObservation)

	eventTypes := rs.GetEventTypes()
	sort.Strings(eventTypes)

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

		ao := &approverObservation{}
		out[eventType] = ao

		approvers, stats, noDiscarderRules, err := rs.GetEventTypeApprovers(eventType, caps)
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
		}
	}

	return out
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
			out[name] = fmt.Sprintf("%s(%v) own=%s(%v)", fmtValue(value), ok, fmtValue(own), ownOk)
		case eval.Variable:
			value, ok := v.GetValue()
			out[name] = fmt.Sprintf("%s(%v)", fmtValue(value), ok)
		default:
			out[name] = fmt.Sprintf("unknown variable type %T", variable)
		}
	}
	return out
}

func buildProcesses(t *testing.T, c *corpusCase) {
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
	case rules.RangeFilterValue:
		return fmt.Sprintf("range(%d,%d)", v.Min, v.Max)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
