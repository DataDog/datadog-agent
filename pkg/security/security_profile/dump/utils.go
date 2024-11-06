// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dump

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/google/uuid"
)

func addRule(expression string, groupID string, opts SECLRuleOpts) *rules.RuleDefinition {
	ruleDef := &rules.RuleDefinition{
		Expression: expression,
		GroupID:    groupID,
		ID:         strings.Replace(uuid.New().String(), "-", "_", -1),
	}
	applyContext(ruleDef, opts)
	if opts.EnableKill {
		applyKillAction(ruleDef)
	}
	return ruleDef
}

func applyKillAction(ruleDef *rules.RuleDefinition) {
	ruleDef.Actions = []*rules.ActionDefinition{
		{
			Kill: &rules.KillDefinition{
				Signal: "SIGKILL",
			},
		},
	}
}

func applyContext(ruleDef *rules.RuleDefinition, opts SECLRuleOpts) {
	var context []string

	if opts.ImageName != "" {
		context = append(context, fmt.Sprintf(`"short_image:%s"`, opts.ImageName))
	}
	if opts.ImageTag != "" {
		context = append(context, fmt.Sprintf(`"image_tag:%s"`, opts.ImageTag))
	}
	if opts.Service != "" {
		context = append(context, fmt.Sprintf(`"service:%s"`, opts.Service))
	}

	if len(context) == 0 {
		return
	}

	ruleDef.Expression = fmt.Sprintf("%s && (%s)", ruleDef.Expression, fmt.Sprintf(`container.tags in [%s]`, strings.Join(context, ", ")))
}

func getGroupID(opts SECLRuleOpts) string {
	groupID := "rules_"
	if len(opts.ImageName) != 0 {
		groupID = fmt.Sprintf("%s%s", groupID, opts.ImageName)
	} else {
		groupID = fmt.Sprintf("%s%s", groupID, strings.Replace(uuid.New().String(), "-", "_", -1)) // It should be unique so that we can target it at least, but ImageName should be always set
	}
	if len(opts.ImageTag) != 0 {
		groupID = fmt.Sprintf("%s_%s", groupID, opts.ImageTag)
	}

	return groupID
}
