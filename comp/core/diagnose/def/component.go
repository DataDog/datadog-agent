// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package diagnose provides the diagnose suite for the agent.
package diagnose

import (
	"encoding/json"
	"sync"

	"github.com/fatih/color"
)

// team: agent-runtimes

type metadataAvailDiagnoseCatalog map[string]func() error

// MetadataAvailCatalog is a set of MetadataAvailDiagnose functions
var MetadataAvailCatalog = make(metadataAvailDiagnoseCatalog)

// RegisterMetadataAvail adds a MetadataAvailDiagnose
func RegisterMetadataAvail(name string, d func() error) {
	if _, ok := MetadataAvailCatalog[name]; !ok {
		MetadataAvailCatalog[name] = d
	}
}

const (
	// CheckDatadog is the suite name for the check-datadog suite
	CheckDatadog = "check-datadog"
	// AutodiscoveryConnectivity is the suite name for the connectivity-datadog-autodiscovery suite
	AutodiscoveryConnectivity = "connectivity-datadog-autodiscovery"
	// CoreEndpointsConnectivity is the suite name for the connectivity-datadog-core-endpoints suite
	CoreEndpointsConnectivity = "connectivity-datadog-core-endpoints"
	// EventPlatformConnectivity is the suite name for the connectivity-datadog-event-platform suite
	EventPlatformConnectivity = "connectivity-datadog-event-platform"
	// PortConflict is the suite name for the port-conflict suite
	PortConflict = "port-conflict"
	// FirewallScan is the suite name for the firewall-scan suite
	FirewallScan = "firewall-scan"
)

// AllSuites is a list of all available suites
var AllSuites = []string{
	CheckDatadog,
	AutodiscoveryConnectivity,
	CoreEndpointsConnectivity,
	EventPlatformConnectivity,
	PortConflict,
	FirewallScan,
}

var catalog *Catalog
var catalogOnce sync.Once

// Suites is a map of suite names to diagnose functions
type Suites map[string]func(Config) []Diagnosis

// Catalog stores the list of registered Diagnose functions
type Catalog struct {
	Suites Suites
}

// Register registers a diagnose function
func (c *Catalog) Register(name string, diagnoseFunc func(Config) []Diagnosis) {
	registeredSuite := false
	for _, suite := range AllSuites {
		if suite == name {
			registeredSuite = true
			break
		}
	}
	if !registeredSuite {
		panic("suite not registered. plase update the AllSuites list")
	}
	c.Suites[name] = diagnoseFunc
}

// GetCatalog returns the global Catalog instance
func GetCatalog() *Catalog {
	catalogOnce.Do(func() {
		catalog = &Catalog{
			Suites: Suites{},
		}
	})
	return catalog
}

// Component is the component type.
type Component interface {
	RunSuites(format string, verbose bool) ([]byte, error)
	RunSuite(suite string, format string, verbose bool) ([]byte, error)
	RunLocalSuite(suites Suites, config Config) (*Result, error)
}

// Status contains the result of the diagnosis
type Status int

// Use explicit constant instead of iota because the same numbers are used
// in Python/CGO calls.
// Change here needs to be reflected in
//    datadog-agent\rtloader\include\rtloader_types.h
//    integrations-core\datadog_checks_base\datadog_checks\base\utils\diagnose.py

// Diagnosis status
const (
	DiagnosisSuccess         Status = 0
	DiagnosisFail            Status = 1
	DiagnosisWarning         Status = 2
	DiagnosisUnexpectedError        = 3
	DiagnosisResultMIN              = DiagnosisSuccess
	DiagnosisResultMAX              = DiagnosisUnexpectedError
)

// ToString returns the string representation of the Result
func (r Status) ToString(colors bool) string {
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

// Counters contains the count of the diagnosis results
type Counters struct {
	Total         int `json:"total,omitempty"`
	Success       int `json:"success,omitempty"`
	Fail          int `json:"fail,omitempty"`
	Warnings      int `json:"warnings,omitempty"`
	UnexpectedErr int `json:"unexpected_error,omitempty"`
}

// Increment increments the count of the diagnosis results
func (c *Counters) Increment(r Status) {
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

// Diagnosis contains the results of the diagnosis
type Diagnosis struct {
	// --------------------------
	// required fields
	// run-time (pass, fail etc)
	Status Status `json:"result"`
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
	// run-time (additional metadata)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MarshalJSON marshals the Diagnose struct to JSON
func (d Diagnosis) MarshalJSON() ([]byte, error) {
	type Alias Diagnosis
	return json.Marshal(&struct {
		Alias
		ResultString string `json:"connectivity_result"`
	}{
		Alias:        (Alias)(d),
		ResultString: d.Status.ToString(false),
	})
}

// Result contains the results of the diagnosis
type Result struct {
	Runs    []Diagnoses `json:"runs"`
	Summary Counters    `json:"summary"`
}

// Diagnoses contains the results of the diagnosis
type Diagnoses struct {
	Name      string      `json:"suite_name"`
	Diagnoses []Diagnosis `json:"diagnoses"`
}

// Config is the configuration for the diagnose
type Config struct {
	Verbose bool
	Include []string
	Exclude []string
}
