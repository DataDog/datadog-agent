package rules

import "github.com/Masterminds/semver/v3"

type RuleDefinition struct {
	ID             string `yaml:"id"`
	Description    string `yaml:"description"`
	Disabled       bool   `yaml:"disabled"`
	TargetType     string `yaml:"target_type"`
	TargetPath     string `yaml:"target_path"`
	TargetName     string `yaml:"target_name"`
	TargetVersion  string `yaml:"target_version"`
	TargetFunction string `yaml:"target_function"`
	TargetOffset   string `yaml:"target_offset"`
	TargetRetprobe bool   `yaml:"target_retprobe"`
	Action         string `yaml:"action"`
	Index          uint64
	Offset         uint64
}

type PolicyDefinition struct {
	Version string            `yaml:"version"`
	Rules   []*RuleDefinition `yaml:"rules"`
}

type Policy struct {
	Name    string
	Source  string
	Version *semver.Version
	Rules   map[string]*RuleDefinition
}
