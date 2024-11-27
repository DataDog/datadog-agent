// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/expr-lang/expr"
)

var (
	datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)
)

const configOrderID = "configuration_order"

type orderConfig struct {
	Order            []string          `json:"order"`
	ScopeExpressions []scopeExpression `json:"scope_expressions"`
}
type scopeExpression struct {
	Expression string `json:"expression"`
	PolicyID   string `json:"config_id"`
}

// Match returns true if the given policy ID matches its scope expression
func (o *orderConfig) Match(policyID string, env map[string]interface{}) (bool, error) {
	var scopeExpression string
	for _, scope := range o.ScopeExpressions {
		if scope.PolicyID == policyID {
			scopeExpression = scope.Expression
			break
		}
	}
	if scopeExpression == "" {
		return false, fmt.Errorf("no scope expression found for policy ID %s", policyID)
	}

	program, err := expr.Compile(scopeExpression, expr.Env(env), expr.AsBool())
	if err != nil {
		return false, err
	}

	output, err := expr.Run(program, env)
	if err != nil {
		return false, err
	}

	boolOutput, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("scope expression %s did not evaluate to a boolean", scopeExpression)
	}

	return boolOutput, nil
}

// getOrderedScopedLayers takes in a Remote Config response and returns the ordered layers
// that match the current scope
// Layers are ordered from the lowest priority to the highest priority so that
// a simple loop can merge them in order
func getOrderedScopedLayers(configs map[string][]byte, env map[string]interface{}) ([][]byte, error) {
	// First unmarshal the order configuration
	var configOrder *orderConfig
	for configID, content := range configs {
		if configID == configOrderID {
			configOrder = &orderConfig{}
			err := json.Unmarshal(content, configOrder)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if configOrder == nil {
		return nil, fmt.Errorf("no order found in the remote config response")
	}

	// Match layers against the scope expressions
	scopedLayers := map[string][]byte{}
	for configID, content := range configs {
		if configID == configOrderID {
			continue
		}

		scopeMatch, err := configOrder.Match(configID, env)
		if err != nil {
			// Don't apply anything if there is an error parsing scope expressions
			return nil, fmt.Errorf("error matching scope expressions: %w", err)
		}
		if scopeMatch {
			scopedLayers[configID] = content
		}
	}

	// Order layers
	layers := make([][]byte, 0)
	for i := len(configOrder.Order) - 1; i >= 0; i-- {
		content, matched := scopedLayers[configOrder.Order[i]]
		if matched {
			layers = append(layers, content)
		}
	}

	return layers, nil
}

func getScopeExprVars(env *env.Env, hostTagsGetter hostTagsGetter) map[string]interface{} {
	return map[string]interface{}{
		"hostname":          env.Hostname,
		"installer_version": version.AgentVersion, // AgentVersion evaluates to the installer version here

		"tags": hostTagsGetter.get(),
	}
}
