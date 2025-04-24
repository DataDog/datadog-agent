// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package common

import (
	"os/user"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (s *Setup) preInstallPackages() (err error) {
	s.addAgentToAdditionalGroups()

	return nil
}

func (s *Setup) addAgentToAdditionalGroups() {
	for _, group := range s.DdAgentAdditionalGroups {
		// Add dd-agent user to additional group for permission reason, in particular to enable reading log files not world readable
		if _, err := user.LookupGroup(group); err != nil {
			log.Infof("Skipping group %s as it does not exist", group)
			continue
		}
		_, err := ExecuteCommandWithTimeout(s, "usermod", "-aG", group, "dd-agent")
		if err != nil {
			s.Out.WriteString("Failed to add dd-agent to group" + group + ": " + err.Error())
			log.Warnf("failed to add dd-agent to group %s:  %v", group, err)
		}
	}
}
