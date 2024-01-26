// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestK8SUserSession(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_k8s_user_session_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/usr/bin/touch", "%s" ] && exec.args_flags == "reference" && exec.user_session.k8s_username != ""`, executable),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	test.Run(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-k8s-user-session-exec")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		var args []string
		var envs []string
		if kind == dockerWrapperType {
			args = []string{"-k8s-user-session", "-user-session-executable", "/usr/bin/touch", "-user-session-open-path", testFile}
		} else if kind == stdWrapperType {
			args = []string{"-k8s-user-session", "-user-session-executable", executable, "-user-session-open-path", testFile}
		}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_k8s_user_session_exec")

			test.validateUserSessionSchema(t, event)

			assert.NotEqual(t, 0, event.ProcessContext.UserSession.ID)
			assert.Equal(t, usersession.UserSessionTypes["k8s"], event.ProcessContext.UserSession.SessionType)
			assert.Equal(t, "qwerty.azerty@datadoghq.com", event.ProcessContext.UserSession.K8SUsername)
			assert.Equal(t, "azerty.qwerty@datadoghq.com", event.ProcessContext.UserSession.K8SUID)
			assert.Equal(t, []string{
				"ABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABC",
				"DEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEF",
			}, event.ProcessContext.UserSession.K8SGroups)
			assert.Equal(t, map[string][]string{
				"my_first_extra_values": {
					"GHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHI",
					"JKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKL",
				},
				"my_second_extra_values": {
					"MNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNO",
					"PQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQR",
					"UVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVW",
					"XYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZ",
				},
			}, event.ProcessContext.UserSession.K8SExtra)
		})
	})
}
