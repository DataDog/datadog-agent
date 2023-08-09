// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnosis TODO comment
package diagnosis

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetadataAvailDiagnose (Metadata availability subcommand)
type MetadataAvailDiagnose func() error
// MetadataAvailDiagnoseCatalog exported type should have comment or be unexported
type MetadataAvailDiagnoseCatalog map[string]MetadataAvailDiagnose

// MetadataAvailCatalog exported var should have comment or be unexported
var MetadataAvailCatalog = make(MetadataAvailDiagnoseCatalog)

// RegisterMetadataAvail exported function should have comment or be unexported
func RegisterMetadataAvail(name string, d MetadataAvailDiagnose) {
	if _, ok := MetadataAvailCatalog[name]; ok {
		log.Warnf("Diagnosis %s already registered, overriding it", name)
	}
	MetadataAvailCatalog[name] = d
}

// --------------------------------
// Diagnose (all subcommand)

// Diagnose interface function
type Diagnose func(Config) []Diagnosis

// Catalog Global list of registered Diagnose functions
var Catalog = make([]Suite, 0)

// Suite Diagnose suite information
type Suite struct {
	SuitName string
	Diagnose Diagnose
}

// Config Diagnose configuration
type Config struct {
	Verbose        bool
	ForceLocal     bool
	RemoteDiagnose bool
	Include        []*regexp.Regexp
	Exclude        []*regexp.Regexp
}

// Result exported type should have comment or be unexported
type Result int

// Use explicit constant instead of iota because the same numbers are used
// in Python/CGO calls.
// Change here needs to be reflected in
//    datadog-agent\rtloader\include\rtloader_types.h
//    integrations-core\datadog_checks_base\datadog_checks\base\utils\diagnose.py

// This const block should have a comment or be unexported
const (
	DiagnosisSuccess         Result = 0
	DiagnosisFail            Result = 1
	DiagnosisWarning         Result = 2
	DiagnosisUnexpectedError        = 3
	DiagnosisResultMIN              = DiagnosisSuccess
	DiagnosisResultMAX              = DiagnosisUnexpectedError
)

// Diagnosis Diagnose result (diagnosis)
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
	Category string
	// static-time (meta typically, description of what being tested)
	Description string
	// run-time (what can be done of what docs need to be consulted to address the issue)
	Remediation string
	// run-time
	RawError error
}

// Diagnoses exported type should have comment or be unexported
type Diagnoses struct {
	SuiteName      string
	SuiteDiagnoses []Diagnosis
}

// Register Add Diagnose suite
func Register(suiteName string, diagnose Diagnose) {
	Catalog = append(Catalog, Suite{
		SuitName: suiteName,
		Diagnose: diagnose,
	})
}
