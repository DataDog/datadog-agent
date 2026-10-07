// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// The Windows file server of the smb-windows cell, seen from the test: it
// prepares the share and the writer over SSH before the Agent pass, starts the
// writer once the Agent runs, reads the writer's files through
// fileserver.ps1, and checks that the server required signing from the
// Agent's session. See windows.go for the VM and its workload.

// windowsSite reads a cell's files on the Windows file server.
type windowsSite struct {
	suite *azureFilesSuite
	c     cell
}

func (s windowsSite) readFiles(_, name string) (string, error) {
	return s.suite.windowsExecute(windowsReadCommand(s.c, name))
}

// windowsServer returns the Windows file server of the environment.
func (suite *azureFilesSuite) windowsServer() *components.RemoteHost {
	suite.T().Helper()
	server := suite.Env().WindowsServer
	require.NotNil(suite.T(), server, "the run has the %s cell, but its environment has no Windows file server", windowsCellName)
	return server
}

// windowsExecute runs a PowerShell command on the Windows file server. An
// error is redacted of every key the test knows, the reader's password
// included, before anything can print it.
func (suite *azureFilesSuite) windowsExecute(command string) (string, error) {
	server := suite.Env().WindowsServer
	if server == nil {
		return "", errors.New("the environment has no Windows file server")
	}
	output, err := server.Execute(command)
	if err != nil {
		return "", errors.New(redactSecrets(err.Error(), suite.knownSecrets()...))
	}
	return output, nil
}

// prepareWindowsServer copies the workload to the Windows file server and
// prepares this run's share and writer, then gives the server's address to
// the cell, whose source the Agent pass configures with it. The reader's
// password goes to the VM as a file over SFTP, which fileserver.ps1 deletes
// once read; no command line, log or output carries it.
func (suite *azureFilesSuite) prepareWindowsServer() {
	c, ok := suite.spec.windowsCell()
	if !ok {
		return
	}
	t := suite.T()
	server := suite.windowsServer()
	password, err := suite.agentAccountKey(c)
	require.NoError(t, err)
	suite.addKnownKey(password)

	files, err := suite.spec.windowsFiles(c)
	require.NoError(t, err)
	for _, dir := range []string{windowsWorkloadDir(), windowsRunDir(), windowsLogsDir(), windowsSecretsDir()} {
		require.NoError(t, server.MkdirAll(dir), "create %s on the Windows file server", dir)
	}
	for path, content := range files {
		_, err := server.WriteFile(path, []byte(content))
		require.NoError(t, err, "copy %s to the Windows file server", path)
	}
	// The secrets directory inherits C:\'s access, which lets every user
	// read it, until protect limits it to SYSTEM, the Administrators and the
	// SSH user.
	output, err := suite.windowsExecute(windowsProtectCommand())
	require.NoError(t, err, "protect the secrets directory of the Windows file server")
	t.Logf("%s: %s", c.name, lastLine(output))
	// prepare deletes the file first thing; this covers a prepare that never
	// ran, or failed before it read the file.
	defer func() {
		if _, err := suite.windowsExecute(windowsForgetPasswordCommand()); err != nil {
			t.Logf("cannot delete the reader's password file on the Windows file server: %v", err)
		}
	}()
	_, err = server.WriteFile(windowsPasswordFile(), []byte(password))
	require.NoError(t, err, "write the reader's password file on the Windows file server")

	output, err = suite.windowsExecute(windowsPrepareCommand(c))
	require.NoError(t, err, "prepare the Windows file server")
	require.False(t, containsAny(output, suite.knownSecrets()), "preparing the Windows file server printed the reader's password")
	t.Logf("%s: %s on %s", c.name, lastLine(output), server.Address)
	suite.spec.setWindowsServerHost(server.Address)
}

// startWindowsWriter starts the writer, the ledger and the appender of the
// Windows file server.
func (suite *azureFilesSuite) startWindowsWriter() {
	c, ok := suite.spec.windowsCell()
	if !ok {
		return
	}
	output, err := suite.windowsExecute(fileServerCommand("start", "Root", windowsRoot, "ShareName", c.shareName, "TaskPrefix", windowsTaskPrefix))
	require.NoError(suite.T(), err, "start the writer of the Windows file server; its logs are under %s", windowsLogsDir())
	suite.T().Logf("%s: %s", c.name, lastLine(output))
}

// windowsSession is one SMB session of the Windows file server, as
// fileserver.ps1 sessions prints it.
type windowsSession struct {
	ClientComputerName string `json:"ClientComputerName"`
	ClientUserName     string `json:"ClientUserName"`
	Dialect            string `json:"Dialect"`
	NumOpens           int64  `json:"NumOpens"`
	SecondsExists      int64  `json:"SecondsExists"`
}

// windowsServerState is what fileserver.ps1 sessions prints: whether the
// server requires signing, and the sessions of the reader account.
type windowsServerState struct {
	RequireSecuritySignature bool             `json:"require_security_signature"`
	EncryptData              bool             `json:"encrypt_data"`
	Sessions                 []windowsSession `json:"sessions"`
}

// parseWindowsServerState decodes the output of fileserver.ps1 sessions.
// PowerShell's ConvertTo-JSON writes a single session as an object rather
// than as an array of one.
func parseWindowsServerState(output string) (windowsServerState, error) {
	var raw struct {
		RequireSecuritySignature bool            `json:"require_security_signature"`
		EncryptData              bool            `json:"encrypt_data"`
		Sessions                 json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(output)), &raw); err != nil {
		return windowsServerState{}, fmt.Errorf("decode the Windows file server's sessions: %w", err)
	}
	state := windowsServerState{RequireSecuritySignature: raw.RequireSecuritySignature, EncryptData: raw.EncryptData}
	sessions := bytes.TrimSpace(raw.Sessions)
	switch {
	case len(sessions) == 0 || bytes.Equal(sessions, []byte("null")):
	case sessions[0] == '{':
		var session windowsSession
		if err := json.Unmarshal(sessions, &session); err != nil {
			return windowsServerState{}, fmt.Errorf("decode the Windows file server's session: %w", err)
		}
		state.Sessions = []windowsSession{session}
	default:
		if err := json.Unmarshal(sessions, &state.Sessions); err != nil {
			return windowsServerState{}, fmt.Errorf("decode the Windows file server's sessions: %w", err)
		}
	}
	return state, nil
}

// checkWindowsServer requires the Windows file server to require signing, and
// to hold a session of the reader account: the Agent read the share as that
// user, over sessions the server only accepts signed.
func (suite *azureFilesSuite) checkWindowsServer(c cell) {
	var state windowsServerState
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		output, err := suite.windowsExecute(fileServerCommand("sessions", "Root", windowsRoot, "UserName", windowsReaderUser))
		require.NoError(collect, err)
		state, err = parseWindowsServerState(output)
		require.NoError(collect, err)
		assert.True(collect, state.RequireSecuritySignature,
			"%s: the Windows file server does not require SMB signing, so the cell does not show that the Agent signs", c.name)
		assert.NotEmpty(collect, state.Sessions,
			"%s: the Windows file server has no SMB session of %s, so the Agent did not read the share as that user", c.name, windowsReaderUser)
	}, time.Minute, 10*time.Second)
	dialects := make([]string, 0, len(state.Sessions))
	for _, session := range state.Sessions {
		dialects = append(dialects, session.ClientComputerName+" SMB "+session.Dialect)
	}
	suite.T().Logf("%s: the Windows file server requires signing; sessions of %s: %v", c.name, windowsReaderUser, dialects)
	if evidence, err := suite.evidenceDir(); err == nil {
		evidence.writeJSON(c.name+"-sessions.json", state)
	}
}

// windowsServerMetadata describes the Windows file server in the run
// metadata.
func windowsServerMetadata(c cell) map[string]any {
	return map[string]any{
		"address":         c.serverHost,
		"share_dir":       windowsShareDir(c),
		"username":        windowsReaderUser,
		"signing":         "required",
		"python_url":      windowsPythonURL(),
		"python_hash":     windowsPythonHashAlgorithm + ":" + windowsPythonHash,
		"scheduled_tasks": windowsTasks,
	}
}

// captureWindowsEvidence keeps the Windows file server's files, its tasks'
// logs and its state. Everything is redacted like the rest of the evidence.
func (suite *azureFilesSuite) captureWindowsEvidence(evidence *evidenceDir, c cell) {
	site := windowsSite{suite: suite, c: c}
	suite.captureWriterFiles(evidence, c, site)
	if rawPeriods, err := site.readFiles(writerContainerName, periodsJournalName); err == nil {
		evidence.write(c.name+"-periods.jsonl", []byte(rawPeriods))
	}
	for _, task := range windowsTasks {
		output, err := suite.windowsExecute(windowsLogsCommand(c, task))
		evidence.writeCommandResult(c.name+"-"+task+".log", output, "", err)
	}
	output, err := suite.windowsExecute(fileServerCommand("describe",
		"Root", windowsRoot, "ShareName", c.shareName, "TaskPrefix", windowsTaskPrefix))
	evidence.writeCommandResult(c.name+"-windows-server.txt", output, "", err)
}
