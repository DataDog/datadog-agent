// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnosis contains types used by the "agent diagnose" command.
package diagnosis

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
type Diagnose func(Config, sender.DiagnoseSenderManager) []Diagnosis

// Suite contains the Diagnose suite information
type Suite struct {
	SuitName string
	Diagnose Diagnose
}

// Config contains the Diagnose configuration
type Config struct {
	Verbose               bool
	RunLocal              bool
	RunningInAgentProcess bool
	Include               []string
	Exclude               []string
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

// Diagnosis contains the results of the diagnosis
type Diagnosis struct {
	// --------------------------
	// required fields

	// run-time (pass, fail etc)
	Result Result
	// static-time (meta typically)
	Name string
	// run-time (actual diagnosis consumable by a user)
	Diagnosis string

	// --------------------------
	// optional fields

	// static-time (meta typically)
	Category string `json:",omitempty"`
	// static-time (meta typically, description of what being tested)
	Description string
	// run-time (what can be done of what docs need to be consulted to address the issue)
	Remediation string
	// run-time
	RawError string
}

// Diagnoses is a collection of Diagnosis
type Diagnoses struct {
	SuiteName      string
	SuiteDiagnoses []Diagnosis
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
