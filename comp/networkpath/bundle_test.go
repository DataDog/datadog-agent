// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package networkpath

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx-mock"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		core.MockBundle(),
		eventplatformmock.MockModule(),
		rdnsquerier.MockModule(),
	)
}
