// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestPam(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_pam",
			Expression: `exec.user_session.ssh_username == "root"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("pam", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			// Effectue un SSH local vers root pour déclencher PAM.
			// - NumberOfPasswordPrompts=0 : pas de prompt, retour rapide.
			// - PreferredAuthentications inclut password/keyboard-interactive si dispo,
			//   mais sans prompt on évite de bloquer le test.
			// - StrictHostKeyChecking/known_hosts désactivés pour l'isolation du test.
			// - Utilise 127.0.0.1 explicitement pour tester l'IP de connexion
			cmd := exec.Command("ssh",
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "PreferredAuthentications=publickey,password,keyboard-interactive",
				"-o", "NumberOfPasswordPrompts=0",
				"root@127.0.0.1", "true",
			)

			// On exécute et on ignore l'erreur : même un échec d'auth doit avoir déclenché PAM.
			_ = cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_pam")
			assert.NotEqual(t, 0, event.ProcessContext.UserSession.ID)
			assert.Equal(t, usersession.UserSessionTypes["ssh"], event.ProcessContext.UserSession.SessionType)
			assert.Equal(t, "root", event.ProcessContext.UserSession.SSHUsername)
			assert.Equal(t, "127.0.0.1", event.ProcessContext.UserSession.SSHHostIP)

		})
	})
}
