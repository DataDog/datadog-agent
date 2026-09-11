// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"fmt"
	"slices"
	"strings"
)

// Runner names accepted by the `run_in` check option.
const (
	// RunnerCoreAgent names the Core Agent's own check scheduler.
	RunnerCoreAgent = "core_agent"
	// RunnerCheckRunner names the out-of-process Agent Check Runner (ACR).
	RunnerCheckRunner = "check_runner"
)

// RunIn is the set of runners that should execute a check instance. It is
// declared in `init_config` or in an instance, and accepts either a single
// runner name or a list of names:
//
//	run_in: check_runner
//	run_in: [core_agent, check_runner]
//
// The Core Agent applies a purely local rule: it runs the instance when
// `run_in` is absent, or when RunnerCoreAgent is one of the named runners. It
// never enumerates the other runners, so a new runner can be introduced
// without changing this code, and naming several runners at once is allowed.
//
// An absent RunIn therefore means "the Core Agent runs it", which is the
// behaviour of every check that predates the option. Exactly one side owns
// that default: a runner other than the Core Agent must execute an instance
// only when it is named explicitly, otherwise every unannotated check would
// run twice.
type RunIn struct {
	set      bool
	parseErr string
	runners  []string
}

// UnmarshalYAML accepts either a scalar runner name or a sequence of them.
//
// It never returns an error. The caller unmarshals a whole instance at once
// (see CheckScheduler.getChecks), and an error here would make the scheduler
// log a warning and drop the instance outright - turning a typo in one field
// into a silent monitoring gap. A value that fits neither shape is recorded
// and reported by evaluateRunIn.
func (r *RunIn) UnmarshalYAML(unmarshal func(interface{}) error) error {
	r.set = true

	var single string
	if err := unmarshal(&single); err == nil {
		r.runners = normalizeRunners([]string{single})
		return nil
	}

	var list []string
	if err := unmarshal(&list); err == nil {
		r.runners = normalizeRunners(list)
		return nil
	}

	r.parseErr = "expected a runner name or a list of runner names"
	return nil
}

// Runners returns the runner names, normalized. It is nil when `run_in` is absent.
func (r RunIn) Runners() []string {
	return slices.Clone(r.runners)
}

// usable reports whether `run_in` was set to a value that names at least one
// runner. An empty string, an empty list and a list of blank entries are all
// errors rather than instructions, and none of them is usable.
func (r RunIn) usable() bool {
	return r.set && r.parseErr == "" && len(r.runners) > 0
}

// invalidReason states why a `run_in` cannot be used, or "" when it can be
// used or was not set at all. The caller appends the consequence.
func (r RunIn) invalidReason() string {
	switch {
	case !r.set || r.usable():
		return ""
	case r.parseErr != "":
		return fmt.Sprintf("run_in is malformed (%s)", r.parseErr)
	default:
		return "run_in names no runner"
	}
}

// normalizeRunners trims and lowercases each name and drops empty entries, so
// that " Core_Agent " and "core_agent" resolve to the same runner.
func normalizeRunners(raw []string) []string {
	runners := make([]string, 0, len(raw))
	for _, name := range raw {
		if normalized := strings.ToLower(strings.TrimSpace(name)); normalized != "" {
			runners = append(runners, normalized)
		}
	}
	return runners
}

// runInDecision is the outcome of applying `run_in` to one check instance.
type runInDecision struct {
	// run reports whether the Core Agent should execute the instance.
	run bool
	// warning is non-empty when the configuration needs operator attention.
	// It is always reported, including when the instance still runs, so that a
	// misconfiguration never looks like a healthy Agent.
	warning string
}

// evaluateRunIn resolves the `run_in` of an instance against the one declared
// in `init_config` - the instance wins when it holds a usable value - and
// decides whether the Core Agent runs that instance.
//
// A value that is set but unusable is an error, not an instruction: it is
// reported and then ignored, so resolution carries on as if it were absent.
// That keeps a broken instance value - `run_in: ""` left by an unresolved
// template variable, say - from deleting monitoring. The exception is an
// unusable instance value over an `init_config` that names runners, which is
// honoured as "no runner" because nothing else explains it.
func evaluateRunIn(initConfig, instance RunIn) runInDecision {
	// One exception: an unusable value in an instance that sits over an
	// init_config which does name runners. Discarding it would silently hand
	// the instance back to the runners the operator was visibly trying to get
	// away from, so it is read as the deliberate - and very odd - statement it
	// can only be, and reported loudly. An instance that executes nowhere is
	// almost never what someone meant to write.
	if instance.set && !instance.usable() && initConfig.usable() {
		return runInDecision{
			run: false,
			warning: fmt.Sprintf("%s, and it overrides init_config (%s): no runner executes this instance",
				instance.invalidReason(), strings.Join(initConfig.runners, ", ")),
		}
	}

	warning := instance.invalidReason()
	if warning == "" {
		warning = initConfig.invalidReason()
	}
	if warning != "" {
		warning += "; ignoring it"
	}

	effective := RunIn{}
	switch {
	case instance.usable():
		effective = instance
	case initConfig.usable():
		effective = initConfig
	}

	switch {
	case !effective.set:
		return runInDecision{run: true, warning: warning}

	case slices.Contains(effective.runners, RunnerCoreAgent):
		return runInDecision{run: true, warning: warning}

	case !slices.Contains(effective.runners, RunnerCheckRunner):
		// Neither runner this Agent knows is named. That is a typo, or a runner
		// this version predates; either way no runner here may claim the
		// instance, so the gap is reported rather than left silent.
		return runInDecision{
			run: false,
			warning: fmt.Sprintf("run_in names no runner known to this Agent (%s); the %s skips this instance",
				strings.Join(effective.runners, ", "), RunnerCoreAgent),
		}

	default:
		return runInDecision{run: false, warning: warning}
	}
}
