// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/hashicorp/go-multierror"
)

const (
	policyExtension = ".policy"
)

var _ PolicyProvider = (*PoliciesDirProvider)(nil)

// PoliciesDirProvider defines a new policy dir provider
type PoliciesDirProvider struct {
	PoliciesDir string
}

// SetOnNewPoliciesReadyCb implements the policy provider interface
func (p *PoliciesDirProvider) SetOnNewPoliciesReadyCb(_ func()) {}

// Start starts the policy dir provider
func (p *PoliciesDirProvider) Start() {}

func (p *PoliciesDirProvider) loadPolicy(filename string, macroFilters []MacroFilter, ruleFilters []RuleFilter) (*Policy, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: filename, Source: PolicyProviderTypeDir, Err: err}
	}
	defer f.Close()

	name := filepath.Base(filename)
	var policyType PolicyType
	if name == DefaultPolicyName {
		policyType = DefaultPolicyType
	} else {
		policyType = CustomPolicyType
	}

	pInfo := &PolicyInfo{
		Name:   name,
		Source: PolicyProviderTypeDir,
		Type:   policyType,
	}

	return LoadPolicy(pInfo, f, macroFilters, ruleFilters)
}

func (p *PoliciesDirProvider) getPolicyFiles() ([]string, error) {
	files, err := os.ReadDir(p.PoliciesDir)
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		switch {
		case files[i].Name() == DefaultPolicyName:
			return true
		case files[j].Name() == DefaultPolicyName:
			return false
		default:
			return files[i].Name() < files[j].Name()
		}
	})

	var policyFiles []string
	for _, policyPath := range files {
		name := policyPath.Name()

		if filepath.Ext(name) == policyExtension {
			filename := filepath.Join(p.PoliciesDir, name)
			policyFiles = append(policyFiles, filename)
		}
	}

	return policyFiles, nil
}

// LoadPolicies implements the policy provider interface
func (p *PoliciesDirProvider) LoadPolicies(macroFilters []MacroFilter, ruleFilters []RuleFilter) ([]*Policy, *multierror.Error) {
	var errs *multierror.Error

	var policies []*Policy

	policyFiles, err := p.getPolicyFiles()
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	slices.Sort(policyFiles)

	// Load and parse policies
	for _, filename := range policyFiles {
		policy, err := p.loadPolicy(filename, macroFilters, ruleFilters)
		if err != nil {
			errs = multierror.Append(errs, err)
		}

		if policy == nil {
			continue
		}

		policies = append(policies, policy)
	}

	return policies, errs
}

// Close stops policy provider interface
func (p *PoliciesDirProvider) Close() error {
	return nil
}

// NewPoliciesDirProvider returns providers for the given policies dir
func NewPoliciesDirProvider(policiesDir string) (*PoliciesDirProvider, error) {
	return &PoliciesDirProvider{
		PoliciesDir: policiesDir,
	}, nil
}

// Type returns the type of policy dir provider
func (p *PoliciesDirProvider) Type() string {
	return PolicyProviderTypeDir
}
