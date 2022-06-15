// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/go-multierror"
)

const policyExtension = ".policy"

var _ PolicyProvider = (*PoliciesDirProvider)(nil)

// PoliciesDirProvider defines a new policy dir provider
type PoliciesDirProvider struct {
	PoliciesDir  string
	agentVersion *semver.Version

	onNewPoliciesReadyCb func()
	cancelFnc            func()
	watcher              *fsnotify.Watcher
	watchedFiles         []string
}

// SetOnNewPoliciesReadyCb implements the policy provider interface
func (p *PoliciesDirProvider) SetOnNewPoliciesReadyCb(cb func()) {
	p.onNewPoliciesReadyCb = cb
}

// Start starts the policy dir provider
func (p *PoliciesDirProvider) Start() {}

func (p *PoliciesDirProvider) loadPolicy(filename string) (*Policy, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: filename, Err: err}
	}
	defer f.Close()

	name := filepath.Base(filename)

	policy, err := LoadPolicy(name, "file", f, p.agentVersion)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	return policy, nil
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
func (p *PoliciesDirProvider) LoadPolicies() ([]*Policy, *multierror.Error) {
	var errs *multierror.Error

	var policies []*Policy

	policyFiles, err := p.getPolicyFiles()
	if err != nil {
		errs = multierror.Append(errs, err)
	}

	// remove oldest watched files
	if p.watcher != nil {
		for _, watched := range p.watchedFiles {
			_ = p.watcher.Remove(watched)
		}
		p.watchedFiles = p.watchedFiles[0:0]
	}

	// Load and parse policies
	for _, filename := range policyFiles {
		policy, err := p.loadPolicy(filename)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			policies = append(policies, policy)

			if p.watcher != nil {
				if err := p.watcher.Add(filename); err != nil {
					errs = multierror.Append(errs, err)
				} else {
					p.watchedFiles = append(p.watchedFiles, filename)
				}
			}
		}
	}

	return policies, errs
}

// Close stops policy provider interface
func (p *PoliciesDirProvider) Close() error {
	if p.cancelFnc != nil {
		p.cancelFnc()
	}

	if p.watcher != nil {
		p.watcher.Close()
	}
	return nil
}

func filesEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func (p *PoliciesDirProvider) watch(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-p.watcher.Events:
				if !ok {
					return
				}

				if event.Op&(fsnotify.Create|fsnotify.Remove) > 0 {
					files, _ := p.getPolicyFiles()
					if !filesEqual(files, p.watchedFiles) {
						p.onNewPoliciesReadyCb()
					}
				} else if event.Op&fsnotify.Write > 0 && filepath.Ext(event.Name) == policyExtension {
					p.onNewPoliciesReadyCb()
				}
			case _, ok := <-p.watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// NewPoliciesDirProvider returns providers for the given policies dir
func NewPoliciesDirProvider(policiesDir string, watch bool, agentVersion *semver.Version) (*PoliciesDirProvider, error) {

	p := &PoliciesDirProvider{
		PoliciesDir: policiesDir,
	}

	if watch {
		var err error
		if p.watcher, err = fsnotify.NewWatcher(); err != nil {
			return nil, err
		}

		if err := p.watcher.Add(policiesDir); err != nil {
			p.watcher.Close()
			return nil, err
		}

		var ctx context.Context
		ctx, p.cancelFnc = context.WithCancel(context.Background())
		go p.watch(ctx)
	}

	return p, nil
}
