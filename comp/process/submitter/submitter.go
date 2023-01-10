// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package submitter

import (
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/types"
)

// submitter implements the Component.
type submitter struct {
}

type dependencies struct {
	fx.In
}

func newSubmitter(deps dependencies) (Component, error) {
	return &submitter{}, nil
}

func (s *submitter) Submit(start time.Time, checkName string, payload *types.Payload) {

}

func (s *submitter) Start() error {
	return nil
}

func (s *submitter) Stop() {

}

func newMock(deps dependencies, t testing.TB) Component {
	// TODO
	return nil
}
