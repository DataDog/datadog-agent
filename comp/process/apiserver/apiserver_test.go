// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestLifecycle(t *testing.T) {
	_ = fxutil.Test[Component](t, fx.Options(
		Module,
		core.MockBundle,
		workloadmeta.Module,
		fx.Supply(context.Background()),
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:6162/config")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}
