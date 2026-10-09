// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretresolution

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/qri-io/jsonpointer"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func (m *module) check() ([]runnerdef.IssueReport, error) {
	if m.deps.Secrets == nil {
		return nil, nil
	}
	var reports []runnerdef.IssueReport
	for _, failure := range m.deps.Secrets.GetResolutionFailures() {
		// Hash the original reference so scrubbing cannot collapse distinct failures.
		h := fnv.New64a()
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", m.deps.Hostname.GetSafe(context.Background()), failure.Origin, failure.Handle, jsonpointer.Pointer(failure.Path).String())
		ctx := map[string]string{
			"handle": failure.Handle, "configuration": failure.OriginName,
			"reason": failure.Reason, "cached": strconv.FormatBool(failure.HasCachedValue),
		}
		for _, key := range []string{"handle", "configuration"} {
			text, err := scrubber.ScrubString(ctx[key])
			if err != nil {
				return nil, err
			}
			ctx[key] = text
		}
		// Scrub decoded map keys before JSON-pointer escaping hides URL credentials.
		for i, token := range failure.Path {
			text, err := scrubber.ScrubString(token)
			if err != nil {
				return nil, err
			}
			failure.Path[i] = text
		}
		ctx["setting_path"] = jsonpointer.Pointer(failure.Path).String()
		reports = append(reports, runnerdef.IssueReport{
			IssueID:   fmt.Sprintf("secret-resolution:%016x", h.Sum64()),
			IssueName: IssueName, Source: "secrets", Context: ctx,
		})
	}
	return reports, nil
}
