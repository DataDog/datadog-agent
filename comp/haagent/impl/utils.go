// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

func leaderStateToRole(isLeader bool) string {
	if isLeader {
		return "leader"
	}
	return "follower"
}
