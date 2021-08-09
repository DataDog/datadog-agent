// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/open-policy-agent/opa/rego"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
)

type regoCheck struct {
	env.Env

	ruleID      string
	description string
	interval    time.Duration

	suiteMeta *compliance.SuiteMeta

	scope           compliance.RuleScope
	resourceHandler resourceReporter

	resources         []compliance.RegoResource
	preparedEvalQuery rego.PreparedEvalQuery
}

func (r *regoCheck) compileQuery(module, query string) error {
	ctx := context.TODO()

	preparedEvalQuery, err := rego.New(
		rego.Query(query),
		rego.Module(fmt.Sprintf("rule_%s.rego", r.ruleID), module),
	).PrepareForEval(ctx)

	if err != nil {
		return err
	}

	r.preparedEvalQuery = preparedEvalQuery

	return nil
}

func (r *regoCheck) Stop() {
}

func (r *regoCheck) Cancel() {
}

func (r *regoCheck) String() string {
	return compliance.CheckName(r.ruleID, r.description)
}

func (r *regoCheck) Configure(config, initConfig integration.Data, source string) error {
	return nil
}

func (r *regoCheck) Interval() time.Duration {
	return r.interval
}

func (r *regoCheck) ID() check.ID {
	return check.ID(r.ruleID)
}

func (r *regoCheck) GetWarnings() []error {
	return nil
}

func (r *regoCheck) GetSenderStats() (check.SenderStats, error) {
	return check.NewSenderStats(), nil
}

func (r *regoCheck) Version() string {
	return r.suiteMeta.Version
}

func (r *regoCheck) ConfigSource() string {
	return r.suiteMeta.Source
}

func (r *regoCheck) IsTelemetryEnabled() bool {
	return false
}

func (r *regoCheck) Run() error {
	if !r.IsLeader() {
		return nil
	}

	var err error

	return err
}
