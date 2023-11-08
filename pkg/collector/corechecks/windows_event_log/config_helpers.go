// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

func compileRegexPatterns(patterns []string) ([]*regexp.Regexp, error) {
	var err error
	res := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		res[i], err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("error compiling regex pattern '%s': %w", pattern, err)
		}
	}
	return res, nil
}

func serverIsLocal(server *string) bool {
	return server == nil ||
		len(*server) == 0 ||
		*server == "localhost" ||
		*server == "127.0.0.1"
}

func evtRPCFlagsFromString(flags string) (uint, error) {
	// NOTE: Keep this in sync with config spec `auth_type`
	switch flags {
	case "default":
		return evtapi.EvtRpcLoginAuthDefault, nil
	case "negotiate":
		return evtapi.EvtRpcLoginAuthNegotiate, nil
	case "kerberos":
		return evtapi.EvtRpcLoginAuthKerberos, nil
	case "ntlm":
		return evtapi.EvtRpcLoginAuthNTLM, nil
	default:
		return 0, fmt.Errorf("invalid RPC auth type: '%s', must be one of default, negotiate, kerberos, ntlm", flags)
	}
}
