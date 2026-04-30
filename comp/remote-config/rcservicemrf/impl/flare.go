// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rcservicemrfimpl

import (
	"bytes"
	"context"
	"fmt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	rcservicemrf "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/def"
	pkgflare "github.com/DataDog/datadog-agent/pkg/flare"
)

func mrfFillFlare(svc rcservicemrf.Component) func(context.Context, flaretypes.FlareBuilder) error {
	return func(_ context.Context, fb flaretypes.FlareBuilder) error {
		state, err := svc.ConfigGetState()
		if err != nil {
			return fmt.Errorf("couldn't get the MRF repositories state: %v", err)
		}

		var buf bytes.Buffer
		pkgflare.PrintRemoteConfigStates(&buf, nil, state)
		return fb.AddFile("remote-config-state-ha.log", buf.Bytes())
	}
}
