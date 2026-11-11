// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// SystemdOperation is one unit action checked by the shared policy.
type SystemdOperation = builtins.SystemdOperation

// SystemServiceAction identifies an operation that may be granted for an exact
// systemd unit. The historical "Service" name is retained for compatibility.
type SystemServiceAction = builtins.SystemServiceAction

const (
	// SystemServiceAllActions grants every supported systemd action for an
	// exact unit, including actions added in future versions.
	SystemServiceAllActions SystemServiceAction = "*"

	SystemServiceRead    = builtins.SystemServiceRead
	SystemServiceClean   = builtins.SystemServiceClean
	SystemServiceStart   = builtins.SystemServiceStart
	SystemServiceStop    = builtins.SystemServiceStop
	SystemServiceReload  = builtins.SystemServiceReload
	SystemServiceRestart = builtins.SystemServiceRestart
	SystemServiceEnable  = builtins.SystemServiceEnable
	SystemServiceDisable = builtins.SystemServiceDisable
)

// SystemServiceControlGrant grants Actions for one exact systemd unit. Service
// may contain any unit type suffix, including .service, .timer, and .socket.
// SystemServiceAllActions grants every action supported by the running version.
type SystemServiceControlGrant struct {
	Service string
	Actions []SystemServiceAction
}

// SystemdControlGrant is an alias for the shared systemd policy grant type.
type SystemdControlGrant = SystemServiceControlGrant

type systemdGrants map[string]map[SystemServiceAction]struct{}

var systemServiceActionOrder = [...]SystemServiceAction{
	SystemServiceRead,
	SystemServiceClean,
	SystemServiceStart,
	SystemServiceStop,
	SystemServiceReload,
	SystemServiceRestart,
	SystemServiceEnable,
	SystemServiceDisable,
}

// AllowedSystemServices configures the units and actions that systemd-aware
// builtins may use. Unit names are matched exactly: for example, "mysql" and
// "mysql.service" are different selectors. Despite the historical API name,
// .timer, .socket, and other unit types are accepted.
//
// Grants without actions are ignored. Invalid services and unsupported actions
// are skipped with a warning. Supported actions are read, clean, start, stop,
// reload, restart, enable, and disable. SystemServiceAllActions expands to all
// supported actions, including actions added in future versions. Duplicate
// units and actions are accepted and combined idempotently.
//
// When not set (default), or when passed an empty slice, every systemd
// operation is denied. This policy is not bypassed by allowing all commands.
func AllowedSystemServices(grants []SystemServiceControlGrant) RunnerOption {
	return func(r *Runner) error {
		allowed, warnings := parseSystemServiceGrants(grants)
		r.allowedSystemServices = allowed
		r.sandboxWarnings = append(r.sandboxWarnings, warnings...)
		return nil
	}
}

func parseSystemServiceGrants(grants []SystemServiceControlGrant) (systemdGrants, []byte) {
	var warnings []byte
	allowed := make(systemdGrants, len(grants))
	for i, grant := range grants {
		if len(grant.Actions) == 0 {
			continue
		}
		if err := validateSystemServiceName(grant.Service); err != nil {
			warning := fmt.Sprintf("AllowedSystemServices: skipping grant %d: %v\n", i, err)
			warnings = append(warnings, warning...)
			continue
		}

		actions := allowed[grant.Service]
		for _, action := range grant.Actions {
			if action == SystemServiceAllActions {
				if actions == nil {
					actions = make(map[SystemServiceAction]struct{}, len(systemServiceActionOrder))
					allowed[grant.Service] = actions
				}
				for _, supported := range systemServiceActionOrder {
					actions[supported] = struct{}{}
				}
				continue
			}
			if !validSystemServiceAction(action) {
				warning := fmt.Sprintf("AllowedSystemServices: skipping unsupported action %q in grant %d for %q\n", action, i, grant.Service)
				warnings = append(warnings, warning...)
				continue
			}
			if actions == nil {
				actions = make(map[SystemServiceAction]struct{}, len(grant.Actions))
				allowed[grant.Service] = actions
			}
			actions[action] = struct{}{}
		}
	}
	return allowed, warnings
}

func validSystemServiceAction(action SystemServiceAction) bool {
	for _, supported := range systemServiceActionOrder {
		if action == supported {
			return true
		}
	}
	return false
}

func (r *Runner) allowedSystemServicesList() []SystemdOperation {
	services := make([]string, 0, len(r.allowedSystemServices))
	for service := range r.allowedSystemServices {
		services = append(services, service)
	}
	sort.Strings(services)

	operations := make([]SystemdOperation, 0, len(services))
	for _, service := range services {
		actions := r.allowedSystemServices[service]
		for _, action := range systemServiceActionOrder {
			if _, ok := actions[action]; ok {
				operations = append(operations, SystemdOperation{
					Service: service,
					Action:  action,
				})
			}
		}
	}
	return operations
}

func (r *Runner) readableSystemServices() []string {
	services := make([]string, 0, len(r.allowedSystemServices))
	for service, actions := range r.allowedSystemServices {
		if _, ok := actions[SystemServiceRead]; ok {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	return services
}

func validateSystemServiceName(service string) error {
	if service == "" {
		return errors.New("system service name must not be empty")
	}
	if len(service) > builtins.MaxSystemServiceNameBytes {
		return fmt.Errorf("system service name exceeds %d bytes", builtins.MaxSystemServiceNameBytes)
	}
	if !utf8.ValidString(service) {
		return errors.New("system service name must be valid UTF-8")
	}
	if strings.ContainsRune(service, '/') || strings.ContainsRune(service, '\\') {
		return fmt.Errorf("system service name %q must not contain a path separator", service)
	}
	if strings.ContainsRune(service, ':') {
		return fmt.Errorf("system service name %q must not contain ':'", service)
	}
	for _, r := range service {
		switch r {
		case '*', '?', '[', ']':
			return fmt.Errorf("system service name %q must not contain a glob pattern", service)
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("system service name %q must not contain whitespace or control characters", service)
		}
	}
	return nil
}

func (r *Runner) authorizeSystemd(operations ...SystemdOperation) error {
	return r.allowedSystemServices.authorize(r.remediationMode, operations...)
}

func (grants systemdGrants) authorize(remediation bool, operations ...SystemdOperation) error {
	if len(operations) == 0 {
		return errors.New("at least one systemd operation is required")
	}

	for _, operation := range operations {
		if err := validateSystemServiceName(operation.Service); err != nil {
			return err
		}
		if !validSystemServiceAction(operation.Action) {
			return fmt.Errorf("unsupported systemd action %q for system service %q", operation.Action, operation.Service)
		}
		if operation.Action != SystemServiceRead && !remediation {
			return fmt.Errorf("systemd action %q requires remediation mode", operation.Action)
		}
		actions := grants[operation.Service]
		if _, ok := actions[operation.Action]; !ok {
			return fmt.Errorf("system service %q is not allowed for action %q", operation.Service, operation.Action)
		}
	}
	return nil
}

func (r *Runner) authorizeSystemServices(action SystemServiceAction, services ...string) error {
	if len(services) == 0 {
		return errors.New("at least one system service is required")
	}
	operations := make([]SystemdOperation, len(services))
	for i, service := range services {
		operations[i] = SystemdOperation{Service: service, Action: action}
	}
	return r.authorizeSystemd(operations...)
}
