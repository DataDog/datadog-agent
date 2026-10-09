// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package secretresolution reports failed secret lookups through Agent Health.
package secretresolution

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/qri-io/jsonpointer"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	store "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Requires connects the reporter to existing resolver and Health components.
type Requires struct {
	fx.In
	Lifecycle fx.Lifecycle
	Config    config.Component
	Log       log.Component
	Hostname  hostnameinterface.Component
	Store     store.Component
	Secrets   secrets.Component `optional:"true"`
}

// Register listens to lookup outcomes; it does not perform or schedule lookups.
func Register(reqs Requires) {
	if reqs.Secrets == nil || !reqs.Config.GetBool("health_platform.enabled") {
		return
	}
	r := &reporter{store: reqs.Store, log: reqs.Log, hostname: reqs.Hostname.GetSafe(context.Background())}
	reqs.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			reqs.Secrets.SetResolutionFailureCallback(r.report)
			return nil
		},
		OnStop: func(context.Context) error {
			reqs.Secrets.SetResolutionFailureCallback(nil)
			return nil
		},
	})
}

type reporter struct {
	store    store.Component
	log      log.Component
	hostname string
}

func (r *reporter) report(failures []secrets.ResolutionFailure, initialLoadComplete bool) {
	current := make(map[string]struct{}, len(failures))
	for _, failure := range failures {
		issue, err := buildIssue(r.hostname, failure)
		if err != nil {
			r.log.Warnf("Unable to build secret resolution issue: %v", err)
			return
		}
		current[issue.Id] = struct{}{}
		if err := r.store.ReportIssue(issue); err != nil {
			r.log.Warnf("Unable to report secret resolution issue: %v", err)
			return
		}
	}
	// Startup's empty snapshot is not evidence of recovery until configs have loaded.
	if !initialLoadComplete {
		return
	}
	for _, id := range r.store.GetActiveIssueIDsByIssueName(IssueName) {
		if _, active := current[id]; !active {
			r.store.ResolveIssue(id)
		}
	}
}

func buildIssue(hostname string, failure secrets.ResolutionFailure) (*healthplatform.Issue, error) {
	// Hash original references so scrubbing cannot collapse distinct failures.
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", hostname, failure.Origin, failure.Handle, jsonpointer.Pointer(failure.Path).String())
	ctx := map[string]string{
		"handle": failure.Handle, "configuration": failure.OriginName,
		"configuration_source": failure.ConfigSource,
		"reason":               failure.Reason, "cached": strconv.FormatBool(failure.HasCachedValue),
	}
	for _, key := range []string{"handle", "configuration", "configuration_source"} {
		text, err := scrubber.ScrubString(ctx[key])
		if err != nil {
			return nil, err
		}
		ctx[key] = text
	}
	// Scrub map keys before JSON-pointer escaping hides URL credentials.
	pointer := jsonpointer.Pointer(slices.Clone(failure.Path))
	for i, token := range pointer {
		text, err := scrubber.ScrubString(token)
		if err != nil {
			return nil, err
		}
		pointer[i] = text
	}
	ctx["setting_path"] = pointer.String()
	issue, err := (&SecretResolutionIssue{}).BuildIssue(ctx)
	if err != nil {
		return nil, err
	}
	issue.Id = fmt.Sprintf("secret-resolution:%016x", h.Sum64())
	return issue, nil
}
