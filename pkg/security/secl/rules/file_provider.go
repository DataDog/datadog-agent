// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const defaultPolicyFile = "default.policy"

// Policy represents a policy file definition
type PolicyDef struct {
	Version string             `yaml:"version"`
	Rules   []*RuleDefinition  `yaml:"rules"`
	Macros  []*MacroDefinition `yaml:"macros"`
}

// PolicyFileProvider defines a file provider
type PolicyFileProvider struct {
	Filename string
	Watch    bool

	onNewPolicyReadyCb func(*Policy)
	cancelFnc          func()
}

func parseDef(name string, def *PolicyDef) (*Policy, *multierror.Error) {
	var errs *multierror.Error

	policy := &Policy{
		Name:    name,
		Source:  "file",
		Version: def.Version,
	}

	for _, macroDef := range def.Macros {
		if macroDef.ID == "" {
			errs = multierror.Append(errs, &ErrMacroLoad{Err: fmt.Errorf("no ID defined for macro with expression `%s`", macroDef.Expression)})
			continue
		}
		if !checkRuleID(macroDef.ID) {
			errs = multierror.Append(errs, &ErrMacroLoad{Definition: macroDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		policy.AddMacro(macroDef)
	}

	for _, ruleDef := range def.Rules {
		if ruleDef.ID == "" {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("no ID defined for rule with expression `%s`", ruleDef.Expression)})
			continue
		}
		if !checkRuleID(ruleDef.ID) {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: fmt.Errorf("ID does not match pattern `%s`", ruleIDPattern)})
			continue
		}

		if ruleDef.Expression == "" && !ruleDef.Disabled {
			errs = multierror.Append(errs, &ErrRuleLoad{Definition: ruleDef, Err: errors.New("no expression defined")})
			continue
		}

		policy.AddRule(ruleDef)
	}

	return policy, errs
}

// LoadPolicy load a policy
func LoadPolicy(name string, reader io.Reader) (*Policy, error) {
	var def PolicyDef

	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	policy, errs := parseDef(name, &def)
	if errs.ErrorOrNil() != nil {
		return nil, errs.ErrorOrNil()
	}

	return policy, nil
}

// LoadPolicy loads a YAML file and returns a new policy
func (p *PolicyFileProvider) LoadPolicy() (*Policy, error) {
	f, err := os.Open(p.Filename)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: p.Filename, Err: err}
	}
	defer f.Close()

	name := filepath.Base(p.Filename)

	policy, err := LoadPolicy(name, f)
	if err != nil {
		return nil, &ErrPolicyLoad{Name: name, Err: err}
	}

	if p.onNewPolicyReadyCb != nil {
		p.onNewPolicyReadyCb(policy)
	}

	if !p.Watch {
		return policy, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	if err := watcher.Add(p.Filename); err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())
	p.cancelFnc = cancelFnc

	go func() {
		defer watcher.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Println("event:", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("modified file:", event.Name)
				}

				p.onNewPolicyReadyCb(policy)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	return policy, nil
}

// Stop implements the policy provider interface
func (p *PolicyFileProvider) Stop() {
	if p.cancelFnc != nil {
		p.cancelFnc()
	}
}

// SetOnNewPolicyReadyCb implements the policy provider interface
func (p *PolicyFileProvider) SetOnNewPolicyReadyCb(cb func(*Policy)) {
	p.onNewPolicyReadyCb = cb
}

// NewPolicyFileProvider returns a new file based policy provider
func NewPolicyFileProvider(filename string, watch bool) *PolicyFileProvider {
	return &PolicyFileProvider{
		Filename: filename,
		Watch:    watch,
	}
}
