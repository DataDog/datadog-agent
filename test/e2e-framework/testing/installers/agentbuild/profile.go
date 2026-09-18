// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

// RequireBinaryRouting checks source capability evidence as well as the bound
// bytes/runtime inventory. Runtime CPU/Python probes alone are not route proof.
func (r Result) RequireBinaryRouting() error {
	if r.Binary == nil || r.Profile == nil {
		return fmt.Errorf("managed binary routing requires a core-source capability receipt; explicitly rebuild with invoke-binary (no automatic re-attestation or source fallback)")
	}
	if err := r.Validate(r.Target); err != nil {
		return err
	}
	return r.Profile.Require(receivers.CoreAgent)
}
