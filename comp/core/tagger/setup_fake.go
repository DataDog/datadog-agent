// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package tagger

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// GetDefaultTagger returns the default Tagger in gvien instance
// For testing only, defaultTagger should not be used directly in production
func GetDefaultTagger(taggerClient Component) Component {
	if taggerClient == nil {
		return nil
	}
	if v, ok := taggerClient.(*TaggerClient); ok {
		return v.defaultTagger
	}
	return nil
}

// SetupFakeTagger provides a fake tagger, for testing only.
func SetupFakeTagger(t *testing.T) *local.FakeTagger {
	// taggerClient is a global variable that is set by the fxutil.Test function
	taggerClient := fxutil.Test[Component](t,
		core.MockBundle(),
		fx.Supply(NewFakeTaggerParams()),
		fx.Provide(func() context.Context { return context.TODO() }),
		Module(),
	)
	fakeTagger := GetDefaultTagger(taggerClient).(*local.FakeTagger)
	return fakeTagger
}

// ResetTagger for testing only
func ResetTagger() {
	UnlockGlobalTaggerClient()
}
