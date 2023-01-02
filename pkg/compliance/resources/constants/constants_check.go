// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package constants

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func resolve(_ context.Context, e env.Env, ruleID string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	if res.Constants == nil {
		return nil, fmt.Errorf("expecting constants resource in constants check")
	}

	constants := res.Constants
	idHash := sha256.New()
	for name := range constants.Values {
		idHash.Write([]byte(name))
	}
	resourceID := fmt.Sprintf("%x", idHash.Sum(nil))

	log.Debugf("%s: running constants check", ruleID)

	vars := eval.VarMap(constants.Values)
	regoInput := eval.RegoInputMap(constants.Values)

	instance := eval.NewInstance(vars, nil, regoInput)
	resolvedInstance := resources.NewResolvedInstance(instance, resourceID, "constants")
	return resolvedInstance, nil
}

func init() {
	resources.RegisterHandler("constants", resolve, nil)
}
