// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnosis contains types used by the "agent diagnose" command.
package diagnosis

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"encoding/json"

	"github.com/fatih/color"
)

// --------------------------------
// Diagnose (Metadata availability subcommand)

// MetadataAvailDiagnose represents a function to fetch the metadata availability
type MetadataAvailDiagnose func() error

// MetadataAvailDiagnoseCatalog is a set of MetadataAvailDiagnose functions
type MetadataAvailDiagnoseCatalog map[string]MetadataAvailDiagnose

// MetadataAvailCatalog is a set of MetadataAvailDiagnose functions
var MetadataAvailCatalog = make(MetadataAvailDiagnoseCatalog)

// RegisterMetadataAvail adds a MetadataAvailDiagnose
func RegisterMetadataAvail(name string, d MetadataAvailDiagnose) {
	if _, ok := MetadataAvailCatalog[name]; ok {
		log.Warnf("Diagnosis %s already registered, overriding it", name)
	}
	MetadataAvailCatalog[name] = d
}

// --------------------------------
// Diagnose (all subcommand)

// Diagnose interface function
type Diagnose func() []Diagnosis

// Suite contains the Diagnose suite information
type Suite struct {
	SuitName string
	Diagnose Diagnose
}

// Config contains the Diagnose configuration
type Config struct {
	Verbose    bool
	RunLocal   bool
	JSONOutput bool
	Include    []string
	Exclude    []string
}

// Result contains the result of the diagnosis
type Result int

// Use explicit constant instead of iota because the same numbers are used
// in Python/CGO calls.
// Change here needs to be reflected in
//    datadog-agent\rtloader\include\rtloader_types.h
//    integrations-core\datadog_checks_base\datadog_checks\base\utils\diagnose.py

// Diagnosis results
const (
	DiagnosisSuccess         Result = 0
	DiagnosisFail            Result = 1
	DiagnosisWarning         Result = 2
	DiagnosisUnexpectedError        = 3
	DiagnosisResultMIN              = DiagnosisSuccess
	DiagnosisResultMAX              = DiagnosisUnexpectedError
)

// ToString returns the string representation of the Result
func (r Result) ToString(colors bool) string {
	switch colors {
	case true:
		switch r {
		case DiagnosisSuccess:
			return color.GreenString("PASS")
		case DiagnosisFail:
			return color.RedString("FAIL")
		case DiagnosisWarning:
			return color.YellowString("WARNING")
		default:
			return color.HiRedString("UNEXPECTED ERROR")
		}
	default:
		switch r {
		case DiagnosisSuccess:
			return "PASS"
		case DiagnosisFail:
			return "FAIL"
		case DiagnosisWarning:
			return "WARNING"
		default:
			return "UNEXPECTED ERROR"
		}
	}
}

// Diagnosis contains the results of the diagnosis
type Diagnosis struct {
	// --------------------------
	// required fields

	// run-time (pass, fail etc)
	Result Result `json:"result"`
	// static-time (meta typically)
	Name string `json:"name"`
	// run-time (actual diagnosis consumable by a user)
	Diagnosis string `json:"diagnosis"`

	// --------------------------
	// optional fields

	// static-time (meta typically)
	Category string `json:"category,omitempty"`
	// static-time (meta typically, description of what being tested)
	Description string `json:"description,omitempty"`
	// run-time (what can be done of what docs need to be consulted to address the issue)
	Remediation string `json:"remediation,omitempty"`
	// run-time
	RawError string `json:"rawerror,omitempty"`
}

// DiagnoseResult contains the results of the diagnose command
type DiagnoseResult struct {
	Diagnoses []Diagnoses `json:"runs"`
	Summary   Counters    `json:"summary"`
}

// Diagnoses is a collection of Diagnosis
type Diagnoses struct {
	SuiteName      string      `json:"suite_name"`
	SuiteDiagnoses []Diagnosis `json:"diagnoses"`
}

// Counters contains the count of the diagnosis results
type Counters struct {
	Total         int `json:"total,omitempty"`
	Success       int `json:"success,omitempty"`
	Fail          int `json:"fail,omitempty"`
	Warnings      int `json:"warnings,omitempty"`
	UnexpectedErr int `json:"unexpected_error,omitempty"`
}

// Increment increments the count of the diagnosis results
func (c *Counters) Increment(r Result) {
	c.Total++

	if r == DiagnosisSuccess {
		c.Success++
	} else if r == DiagnosisFail {
		c.Fail++
	} else if r == DiagnosisWarning {
		c.Warnings++
	} else {
		c.UnexpectedErr++
	}
}

// Catalog stores the list of registered Diagnose functions
type Catalog struct {
	suites []Suite
}

// NewCatalog returns a new Catalog instance
func NewCatalog() *Catalog {
	return &Catalog{}
}

// Register registers the given Diagnose function
func (c *Catalog) Register(suiteName string, diagnose Diagnose) {
	c.suites = append(c.suites, Suite{
		SuitName: suiteName,
		Diagnose: diagnose,
	})
}

// GetSuites returns the list of registered Diagnose functions
func (c *Catalog) GetSuites() []Suite {
	return c.suites
}

// MarshalJSON marshals the Diagnose struct to JSON
func (d Diagnosis) MarshalJSON() ([]byte, error) {
	type Alias Diagnosis
	return json.Marshal(&struct {
		Alias
		ResultString string `json:"connectivity_result"`
	}{
		Alias:        (Alias)(d),
		ResultString: d.Result.ToString(false),
	})
}
