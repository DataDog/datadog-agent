// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// MacroID represents the ID of a macro
type MacroID = string

// CombinePolicy represents the policy to use to combine rules and macros
type CombinePolicy = string

// Combine policies
const (
	NoPolicy       CombinePolicy = ""
	MergePolicy    CombinePolicy = "merge"
	OverridePolicy CombinePolicy = "override"
)

// OverrideField defines a combine field
type OverrideField = string

const (
	// OverrideAllFields used to override all the fields
	OverrideAllFields OverrideField = "all"
	// OverrideActionFields used to override the actions
	OverrideActionFields OverrideField = "actions"
	// OverrideEveryField used to override the every field
	OverrideEveryField OverrideField = "every"
	// OverrideTagsField used to override the tags
	OverrideTagsField OverrideField = "tags"
	// OverrideProductTagsField used to override the product_tags field
	OverrideProductTagsField OverrideField = "product_tags"
)

// OverrideOptions defines combine options
type OverrideOptions struct {
	Fields []OverrideField `yaml:"fields,omitempty" json:"fields,omitempty" jsonschema:"enum=all,enum=expression,enum=actions,enum=every,enum=tags"`
}

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID                     MacroID       `yaml:"id" json:"id"`
	Expression             string        `yaml:"expression,omitempty" json:"expression,omitempty" jsonschema:"oneof_required=MacroWithExpression"`
	Description            string        `yaml:"description,omitempty" json:"description,omitempty"`
	AgentVersionConstraint string        `yaml:"agent_version,omitempty" json:"agent_version,omitempty"`
	Filters                []string      `yaml:"filters,omitempty" json:"filters,omitempty"`
	Values                 []string      `yaml:"values,omitempty" json:"values,omitempty" jsonschema:"oneof_required=MacroWithValues"`
	Combine                CombinePolicy `yaml:"combine,omitempty" json:"combine,omitempty" jsonschema:"enum=merge,enum=override"`
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID                     RuleID                 `yaml:"id,omitempty" json:"id"`
	Version                string                 `yaml:"version,omitempty" json:"version,omitempty"`
	Expression             string                 `yaml:"expression,omitempty" json:"expression,omitempty"`
	Description            string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Tags                   map[string]string      `yaml:"tags,omitempty" json:"tags,omitempty"`
	ProductTags            []string               `yaml:"product_tags,omitempty" json:"product_tags,omitempty"`
	AgentVersionConstraint string                 `yaml:"agent_version,omitempty" json:"agent_version,omitempty"`
	Filters                []string               `yaml:"filters,omitempty" json:"filters,omitempty"`
	Disabled               bool                   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Combine                CombinePolicy          `yaml:"combine,omitempty" json:"combine,omitempty" jsonschema:"enum=override"`
	OverrideOptions        OverrideOptions        `yaml:"override_options,omitempty" json:"override_options,omitzero,omitempty"`
	Actions                []*ActionDefinition    `yaml:"actions,omitempty" json:"actions,omitempty"`
	Every                  *HumanReadableDuration `yaml:"every,omitempty" json:"every,omitempty"`
	RateLimiterToken       []string               `yaml:"limiter_token,omitempty" json:"limiter_token,omitempty"`
	Silent                 bool                   `yaml:"silent,omitempty" json:"silent,omitempty"`
	GroupID                string                 `yaml:"group_id,omitempty" json:"group_id,omitempty"`
	Priority               int                    `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// GetTag returns the tag value associated with a tag key
func (rd *RuleDefinition) GetTag(tagKey string) (string, bool) {
	tagValue, ok := rd.Tags[tagKey]
	if ok {
		return tagValue, true
	}
	return "", false
}

// ActionName defines an action name
type ActionName = string

const (
	// KillAction name of the kill action
	KillAction ActionName = "kill"
	// SetAction name of the set action
	SetAction ActionName = "set"
	// CoreDumpAction name of the core dump action
	CoreDumpAction ActionName = "coredump"
	// HashAction name of the hash action
	HashAction ActionName = "hash"
	// LogAction name of the log action
	LogAction ActionName = "log"
	// NetworkFilterAction name of the network filter action
	NetworkFilterAction ActionName = "network_filter"
)

// ActionDefinitionInterface is an interface that describes a rule action section
type ActionDefinitionInterface interface {
	PreCheck(opts PolicyLoaderOpts) error
	IsActionSupported(eventTypeEnabled map[eval.EventType]bool) error
}

// ActionDefinition describes a rule action section
type ActionDefinition struct {
	Filter        *string                  `yaml:"filter,omitempty" json:"filter,omitempty"`
	Set           *SetDefinition           `yaml:"set,omitempty" json:"set,omitempty" jsonschema:"oneof_required=SetAction"`
	Kill          *KillDefinition          `yaml:"kill,omitempty" json:"kill,omitempty" jsonschema:"oneof_required=KillAction"`
	CoreDump      *CoreDumpDefinition      `yaml:"coredump,omitempty" json:"coredump,omitempty" jsonschema:"oneof_required=CoreDumpAction"`
	Hash          *HashDefinition          `yaml:"hash,omitempty" json:"hash,omitempty" jsonschema:"oneof_required=HashAction"`
	Log           *LogDefinition           `yaml:"log,omitempty" json:"log,omitempty" jsonschema:"oneof_required=LogAction"`
	NetworkFilter *NetworkFilterDefinition `yaml:"network_filter,omitempty" json:"network_filter,omitempty" jsonschema:"oneof_required=NetworkFilterAction"`
}

// Name returns the name of the action
func (a *ActionDefinition) Name() ActionName {
	switch {
	case a.Set != nil:
		return SetAction
	case a.Kill != nil:
		return KillAction
	case a.CoreDump != nil:
		return CoreDumpAction
	case a.Hash != nil:
		return HashAction
	case a.Log != nil:
		return LogAction
	case a.NetworkFilter != nil:
		return NetworkFilterAction
	default:
		return ""
	}
}

func (a *ActionDefinition) getCandidateActions() map[string]ActionDefinitionInterface {
	return map[string]ActionDefinitionInterface{
		SetAction:           a.Set,
		KillAction:          a.Kill,
		HashAction:          a.Hash,
		CoreDumpAction:      a.CoreDump,
		LogAction:           a.Log,
		NetworkFilterAction: a.NetworkFilter,
	}
}

// PreCheck returns an error if the action is invalid
func (a *ActionDefinition) PreCheck(opts PolicyLoaderOpts) error {
	candidateActions := a.getCandidateActions()
	actions := 0

	for _, action := range candidateActions {
		if !reflect.ValueOf(action).IsNil() {
			if err := action.PreCheck(opts); err != nil {
				return err
			}
			actions++
		}
	}

	if actions == 0 {
		return fmt.Errorf("either %+v section of an action must be specified", maps.Keys(candidateActions))
	}

	if actions > 1 {
		return errors.New("only one action can be specified")
	}

	return nil
}

// IsActionSupported returns true if the action is supported given a list of enabled event type
func (a *ActionDefinition) IsActionSupported(eventTypeEnabled map[eval.EventType]bool) error {
	candidateActions := a.getCandidateActions()

	for _, action := range candidateActions {
		if !reflect.ValueOf(action).IsNil() {
			if err := action.IsActionSupported(eventTypeEnabled); err != nil {
				return err
			}
		}
	}
	return nil
}

// Scope describes the scope variables
type Scope string

// DefaultActionDefinition describes the base type for action
type DefaultActionDefinition struct{}

// PreCheck returns an error if the action is invalid before parsing
func (a *DefaultActionDefinition) PreCheck(_ PolicyLoaderOpts) error {
	return nil
}

// IsActionSupported returns true if the action is supported with the provided set of enabled event types
func (a *DefaultActionDefinition) IsActionSupported(_ map[eval.EventType]bool) error {
	return nil
}

// SetDefinition describes the 'set' section of a rule action
type SetDefinition struct {
	DefaultActionDefinition `yaml:"-" json:"-"`
	Name                    string                 `yaml:"name,omitempty" json:"name"`
	Value                   interface{}            `yaml:"value,omitempty" json:"value,omitempty" jsonschema:"oneof_required=SetWithValue,oneof_type=string;integer;boolean;array"`
	DefaultValue            interface{}            `yaml:"default_value,omitempty" json:"default_value,omitempty" jsonschema:"oneof_type=string;integer;boolean;array"`
	Field                   string                 `yaml:"field,omitempty" json:"field,omitempty" jsonschema:"oneof_required=SetWithField"`
	Expression              string                 `yaml:"expression,omitempty" json:"expression,omitempty" jsonschema:"oneof_required=SetWithExpression"`
	Append                  bool                   `yaml:"append,omitempty" json:"append,omitempty"`
	Scope                   Scope                  `yaml:"scope,omitempty" json:"scope,omitempty" jsonschema:"enum=process,enum=container,enum=cgroup"`
	ScopeField              string                 `yaml:"scope_field,omitempty" json:"scope_field,omitempty"`
	Size                    int                    `yaml:"size,omitempty" json:"size,omitempty"`
	TTL                     *HumanReadableDuration `yaml:"ttl,omitempty" json:"ttl,omitempty"`
	Private                 bool                   `yaml:"private,omitempty" json:"private,omitempty"`
	Inherited               bool                   `yaml:"inherited,omitempty" json:"inherited,omitempty"`
}

// PreCheck returns an error if the set action is invalid
func (s *SetDefinition) PreCheck(_ PolicyLoaderOpts) error {
	if s.Name == "" {
		return errors.New("variable name is empty")
	}

	if s.DefaultValue != nil {
		if defaultValueType, valueType := reflect.TypeOf(s.DefaultValue), reflect.TypeOf(s.Value); valueType != nil && defaultValueType != valueType {
			return fmt.Errorf("'default_value' and 'value' must be of the same type (%s != %s)", defaultValueType, valueType)
		}
	}

	if (s.Value == nil && s.Expression == "" && s.Field == "") ||
		(s.Expression != "" && s.Field != "") ||
		(s.Field != "" && s.Value != nil) ||
		(s.Value != nil && s.Expression != "") {
		return errors.New("either 'value', 'field' or 'expression' must be specified")
	}

	if s.Expression != "" && s.DefaultValue == nil && s.Value == nil {
		return fmt.Errorf("failed to infer type for variable '%s', please set 'default_value'", s.Name)
	}

	if s.Inherited && s.Scope != ScopeProcess {
		return errors.New("only variables scoped to process can be marked as inherited")
	}

	if len(s.ScopeField) > 0 && s.Scope != ScopeProcess {
		return errors.New("only variables scoped to process can have a custom scope_field")
	}

	return nil
}

// KillDefinition describes the 'kill' section of a rule action
type KillDefinition struct {
	DefaultActionDefinition   `yaml:"-" json:"-"`
	Signal                    string `yaml:"signal" json:"signal" jsonschema:"description=A valid signal name,example=SIGKILL,example=SIGTERM"`
	Scope                     string `yaml:"scope,omitempty" json:"scope,omitempty" jsonschema:"enum=process,enum=container,enum=cgroup"`
	DisableContainerDisarmer  bool   `yaml:"disable_container_disarmer,omitempty" json:"disable_container_disarmer,omitempty" jsonschema:"description=Set to true to disable the rule kill action automatic container disarmer safeguard"`
	DisableExecutableDisarmer bool   `yaml:"disable_executable_disarmer,omitempty" json:"disable_executable_disarmer,omitempty" jsonschema:"description=Set to true to disable the rule kill action automatic executable disarmer safeguard"`
}

// PreCheck returns an error if the kill action is invalid
func (k *KillDefinition) PreCheck(opts PolicyLoaderOpts) error {
	if opts.DisableEnforcement {
		return errors.New("'kill' action is disabled globally")
	}

	if k.Signal == "" {
		return errors.New("a valid signal has to be specified to the 'kill' action")
	}

	if _, found := model.SignalConstants[k.Signal]; !found {
		return fmt.Errorf("unsupported signal '%s'", k.Signal)
	}

	return nil
}

// CoreDumpDefinition describes the 'coredump' action
type CoreDumpDefinition struct {
	DefaultActionDefinition `yaml:"-" json:"-"`
	Process                 bool `yaml:"process,omitempty" json:"process,omitempty" jsonschema:"anyof_required=CoreDumpWithProcess"`
	Mount                   bool `yaml:"mount,omitempty" json:"mount,omitempty" jsonschema:"anyof_required=CoreDumpWithMount"`
	Dentry                  bool `yaml:"dentry,omitempty" json:"dentry,omitempty" jsonschema:"anyof_required=CoreDumpWithDentry"`
	NoCompression           bool `yaml:"no_compression,omitempty" json:"no_compression,omitempty"`
}

// HashDefinition describes the 'hash' section of a rule action
type HashDefinition struct {
	DefaultActionDefinition `yaml:"-" json:"-"`
	Field                   string `yaml:"field,omitempty" json:"field,omitempty"`
	MaxFileSize             int64  `yaml:"max_file_size,omitempty" json:"max_file_size,omitempty"`
}

// PostCheck returns an error if the hash action is invalid after parsing
func (h *HashDefinition) PostCheck(rule *eval.Rule) error {
	ruleEventType, err := rule.GetEventType()
	if err != nil {
		return err
	}

	if h.Field == "" {
		switch ruleEventType {
		case "open":
			h.Field = "open.file"
		case "exec":
			h.Field = "exec.file"
		default:
			return fmt.Errorf("`field` attribute is mandatory for '%s' rules", ruleEventType)
		}
	}

	var eventType model.EventType
	ev := model.NewFakeEvent()
	eventType, err = model.ParseEvalEventType(ruleEventType)
	if err != nil {
		return err
	}

	ev.Type = uint32(eventType)
	if err := ev.ValidateFileField(h.Field); err != nil {
		return err
	}

	// check that the field is compatible with the rule event type
	fieldPathForMetadata := h.Field + ".path"
	fieldEventType, _, _, _, err := ev.GetFieldMetadata(fieldPathForMetadata)
	if err != nil {
		return fmt.Errorf("failed to get event type for field '%s': %w", fieldPathForMetadata, err)
	}

	// if the field has an event type, we check it matches the rule event type
	if fieldEventType != "" && fieldEventType != ruleEventType {
		return fmt.Errorf("field '%s' is not compatible with '%s' rules", h.Field, ruleEventType)
	}

	return nil
}

// LogDefinition describes the 'log' section of a rule action
type LogDefinition struct {
	DefaultActionDefinition `yaml:"-" json:"-"`
	Level                   string `yaml:"level,omitempty" json:"level,omitempty"`
	Message                 string `yaml:"message,omitempty" json:"message,omitempty"`
}

// PreCheck returns an error if the log action is invalid
func (l *LogDefinition) PreCheck(_ PolicyLoaderOpts) error {
	if l.Level == "" {
		return errors.New("a valid log level must be specified to the 'log' action")
	}

	return nil
}

// NetworkFilterDefinition describes the 'network_filter' section of a rule action
type NetworkFilterDefinition struct {
	DefaultActionDefinition `yaml:"-" json:"-"`
	BPFFilter               string `yaml:"filter,omitempty" json:"filter,omitempty"`
	Policy                  string `yaml:"policy,omitempty" json:"policy,omitempty"`
	Scope                   string `yaml:"scope,omitempty" json:"scope,omitempty" jsonschema:"enum=process,enum=cgroup"`
}

// PreCheck returns an error if the network filter action is invalid
func (n *NetworkFilterDefinition) PreCheck(_ PolicyLoaderOpts) error {
	if n.BPFFilter == "" {
		return errors.New("a valid BPF filter must be specified to the 'network_filter' action")
	}

	// default scope to process
	if n.Scope != "" && n.Scope != "process" && n.Scope != "cgroup" {
		return fmt.Errorf("invalid scope '%s'", n.Scope)
	}

	return nil
}

// IsActionSupported returns true if the action is supported with the provided set of enabled event types
func (n *NetworkFilterDefinition) IsActionSupported(eventTypeEnabled map[eval.EventType]bool) error {
	if !eventTypeEnabled[model.RawPacketFilterEventType.String()] {
		return fmt.Errorf("network_filter action requires %s event type", model.RawPacketActionEventType)
	}
	return nil
}

// OnDemandHookPoint represents a hook point definition
type OnDemandHookPoint struct {
	Name      string
	IsSyscall bool
	Args      []HookPointArg
}

// HookPointArg represents the definition of a hook point argument
type HookPointArg struct {
	N    int
	Kind string
}

// PolicyDef represents a policy file definition
type PolicyDef struct {
	Version string `yaml:"version,omitempty" json:"version"`
	// Type is the type of content served by the policy (e.g. "policy" for a default policy, "content_pack" or empty for others)
	Type            string             `yaml:"type,omitempty" json:"type,omitempty"`
	ReplacePolicyID string             `yaml:"replace_policy_id,omitempty" json:"replace_policy_id,omitempty"`
	Macros          []*MacroDefinition `yaml:"macros,omitempty" json:"macros,omitempty"`
	Rules           []*RuleDefinition  `yaml:"rules" json:"rules"`
}

// HumanReadableDuration represents a duration that can unmarshalled from YAML from a human readable format (like `10m`)
// or from a regular integer
type HumanReadableDuration struct {
	time.Duration
}

// GetDuration returns the duration embedded in the HumanReadableDuration, or 0 if nil
func (d *HumanReadableDuration) GetDuration() time.Duration {
	if d == nil {
		return 0
	}
	return d.Duration
}

// MarshalYAML marshals a duration to a human readable format
func (d *HumanReadableDuration) MarshalYAML() (interface{}, error) {
	if d == nil || d.Duration == 0 {
		return nil, nil
	}
	return d.String(), nil
}

// UnmarshalYAML unmarshals a duration from a human readable format or from an integer
func (d *HumanReadableDuration) UnmarshalYAML(n *yaml.Node) error {
	var v interface{}
	if err := n.Decode(&v); err != nil {
		return err
	}
	switch value := v.(type) {
	case int:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid duration: (yaml type: %T)", v)
	}
}

// MarshalJSON marshals a duration to a human readable format
func (d *HumanReadableDuration) MarshalJSON() ([]byte, error) {
	if d == nil {
		return nil, nil
	}
	return json.Marshal(d.GetDuration())
}

// UnmarshalJSON unmarshals a duration from a human readable format or from an integer
func (d *HumanReadableDuration) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		// JSON numbers are unmarshaled as float64 by default
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid duration: (json type: %T)", v)
	}
}

var _ yaml.Marshaler = (*HumanReadableDuration)(nil)
var _ yaml.Unmarshaler = (*HumanReadableDuration)(nil)
var _ json.Marshaler = (*HumanReadableDuration)(nil)
var _ json.Unmarshaler = (*HumanReadableDuration)(nil)
