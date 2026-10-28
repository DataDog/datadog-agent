// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	autoscalingstore "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// renderWholeStatus builds the real status component with this package's
// provider registered, and returns the full text output.
func renderWholeStatus(t *testing.T) string {
	t.Helper()

	conf := config.NewMock(t)
	conf.SetInTest("autoscaling.workload.enabled", true)

	comp := fxutil.Test[status.Component](t, fx.Options(
		fx.Provide(func() config.Component { return conf }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		// The common header provider calls this unconditionally.
		fx.Supply(status.Params{PythonVersionGetFunc: func() string { return "n/a" }}),
		fx.Supply(status.NewInformationProvider(Provider{})),
		statusimpl.Module(),
	))

	out, err := comp.GetStatus("text", false)
	require.NoError(t, err)
	return string(out)
}

// TestWorkloadAutoscalingStatusIsInTheAutoscalingGroup pins the shape of the
// rendered section: the framework supplies the "Autoscaling" group heading, and
// this provider contributes a "Workload" sub-heading under it, leaving room for
// cluster autoscaling to join the same group later.
func TestWorkloadAutoscalingStatusIsInTheAutoscalingGroup(t *testing.T) {
	resetStatus(t)

	store := autoscalingstore.NewStore[model.PodAutoscalerInternal]()
	for _, id := range []string{"ns/one", "ns/two"} {
		item, _ := store.Get(id)
		item.Upsert(model.PodAutoscalerInternal{}, "test")
		item.Release()
	}
	InitStatus(store, func() bool { return true })

	out := renderWholeStatus(t)
	section := sectionOf(out, "Autoscaling")
	t.Logf("\n%s", section)
	require.NotEmpty(t, section, "the Autoscaling section must be rendered")
	// The group heading comes from the framework, the sub-heading from this
	// provider's template, in that order. Pinning the exact lines also pins the
	// framework's banner format, which is the point: this test exists to catch
	// the section moving or losing its grouping.
	want := []string{
		"===========",
		"Autoscaling",
		"===========",
		"",
		"  Workload",
		"  --------",
		"  DatadogPodAutoscalers: 2",
		"  Leader: true",
	}
	lines := strings.Split(section, "\n")
	require.GreaterOrEqual(t, len(lines), len(want), "section rendered short:\n%s", section)
	assert.Equal(t, want, lines[:len(want)])
}

// sectionOf returns the block of the status output belonging to one section.
// A heading is three lines -- dashes, name, dashes -- so the search for the next
// section has to start past the closing dashes of this one.
func sectionOf(out, name string) string {
	marker := strings.Repeat("=", len(name))
	header := marker + "\n" + name + "\n" + marker
	start := strings.Index(out, header)
	if start == -1 {
		return ""
	}
	rest := out[start:]
	if next := strings.Index(rest[len(header):], "\n="); next != -1 {
		return rest[:len(header)+next]
	}
	return rest
}
