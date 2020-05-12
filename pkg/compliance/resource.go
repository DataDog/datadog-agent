// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

type ResourceKind int

// Resource describes supported resource types observed by a Rule
type Resource struct {
	File    *File    `yaml:"file,omitempty"`
	Process *Process `yaml:"process,omitempty"`
	Group   *Group   `yaml:"group,omitempty"`
	Command *Command `yaml:"command,omitempty"`
	Audit   *Audit   `yaml:"audit,omitempty"`
	API     *API     `yaml:"api,omitempty"`
}

// File describes a file resource
type File struct {
	Path     string    `yaml:"path,omitempty"`
	PathFrom ValueFrom `yaml:"pathFrom,omitempty"`
	Glob     string    `yaml:"glob,omitempty"`

	Filter []FileFilter `yaml:"filter,omitempty"`

	Report Report `yaml:"report,omitempty"`
}

// Process describes a process resource
type Process struct {
	Name string `yaml:"name"`

	Report Report `yaml:"report,omitempty"`
}

// Group describes a group membership resource
type Group struct {
	Name string `yaml:"name"`

	Report Report `yaml:"report,omitempty"`
}

// Command describes a command resource usually reporting exit code or output
type Command struct {
	Run string `yaml:"run"`

	Filter []CommandFilter `yaml:"filter,omitempty"`

	Report Report `yaml:"report,omitempty"`
}

// Audit describes an audited file resource
type Audit struct {
	Path     string    `yaml:"path,omitempty"`
	PathFrom ValueFrom `yaml:"pathFrom,omitempty"`

	Report Report `yaml:"report,omitempty"`
}

type API struct {
	Kind string `yaml:"kind"`
	Get  string `yaml:"get,omitempty"`

	Vars APIVars `yaml:"vars,omitempty"`

	Filter []APIFilter `yaml:"filter,omitempty"`

	Report Report `yaml:"report,omitempty"`
}

// ValueFrom provides a lookup list for substitution of a value in a Resource
type ValueFrom []ValueSource

// ValueSource provides a single lookup option for value substitution in a Resource
type ValueSource struct {
	Command string           `yaml:"command,omitempty"`
	File    ValueFromFile    `yaml:"file,omitempty"`
	Process ValueFromProcess `yaml:"process,omitempty"`
}

// ValueFromFile describes a value taken from properties of a file
type ValueFromFile struct {
	Path     string `yaml:"path"`
	Property string `yaml:"property"`
}

// ValueFromProcess describes a value taken from attributes of a process
type ValueFromProcess struct {
	Name string `yaml:"name"`
	Flag string `yaml:"flag"`
}

// Report defines a set of reported fields which are sent in a RuleEvent
type Report []ReportedField

// ReportedField defines options for reporting various attributes of observed resources
type ReportedField struct {
	Attribute string `yaml:"attribute,omitempty"`
	Name      string `yaml:"name,omitempty"`
	JSONPath  string `yaml:"jsonpath,omitempty"`
	Var       string `yaml:"var,omitempty"`
	As        string `yaml:"as,omitempty"`
	Value     string `yaml:"value,omitempty"`
}

// FileFilter specifies filtering options for including or excluding a File from reporting
type FileFilter struct {
	Include *FileCondition `yaml:"include,omitempty"`
	Exclude *FileCondition `yaml:"exclude,omitempty"`
}

// FileCondition specifies a condition to include or exclude a File from reporting
type FileCondition struct {
	Owner          string `yaml:"owner,omitempty"`
	MostPermissive bool   `yaml:"mostPermissive"`
}

// CommandFilter specifies filtering options to include or exclude a Command from reporting
type CommandFilter struct {
	Include *CommandCondition `yaml:"include,omitempty"`
	Exclude *CommandCondition `yaml:"exclude,omitempty"`
}

// CommandCondition specicies conditions to include or exclude a Command from reporting
type CommandCondition struct {
	ExitCode *int `yaml:"exitCode,omitempty"`
}

// APIVars defines a list of variables substituted in generic API resource endpoint queries
type APIVars []APIVar

// APIVar defines a variable substitution in generic API resource endpoint query
type APIVar struct {
	Name  string       `yaml:"name"`
	List  *APIVarValue `yaml:"enumerate,omitempty"`
	Value *APIVarValue `yaml:"value,omitempty"`
}

// APIVarValue defines how an API variable is retrieved
type APIVarValue struct {
	Get      string `yaml:"get"`
	JSONPath string `yaml:"jsonpath"`
}

// APIFilter specifies filtering options to include or exclude an API resource
type APIFilter struct {
	Include *APICondition `yaml:"include,omitempty"`
	Exclude *APICondition `yaml:"exclude,omitempty"`
}

// APICondition specifies a filtering condition to include or exclude an API resource
type APICondition struct {
	JSONPath string `yaml:"jsonpath"`
	Exists   bool   `yaml:"exists"`
}
