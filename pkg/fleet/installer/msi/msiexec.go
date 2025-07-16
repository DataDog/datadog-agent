// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package msi contains helper functions to work with msi packages.
//
// The package provides automatic retry functionality for MSI operations using exponential backoff
// to handle transient errors, particularly exit code 1618 (ERROR_INSTALL_ALREADY_RUNNING)
// which occurs when another MSI installation is in progress.
package msi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/cenkalti/backoff/v5"
	"golang.org/x/sys/windows"
)

// exitCodeError interface for errors that have an exit code
//
// Used in place of exec.ExitError to enable mocks for testing.
type exitCodeError interface {
	error
	ExitCode() int
}

var (
	system32Path = `C:\Windows\System32`
	msiexecPath  = filepath.Join(system32Path, "msiexec.exe")
)

func init() {
	system32Path, err := windows.KnownFolderPath(windows.FOLDERID_System, 0)
	if err == nil {
		msiexecPath = filepath.Join(system32Path, "msiexec.exe")
	}
}

type msiexecArgs struct {
	// target should be either a full path to a MSI, an URL to a MSI or a product code.
	target string

	// msiAction should be "/i" for installation, "/x" for uninstallation etc...
	msiAction string

	// logFile should be a full local path where msiexec will write the installation logs.
	// If nothing is specified, a random, temporary file is used.
	logFile             string
	ddagentUserName     string
	ddagentUserPassword string

	// additionalArgs are further args that can be passed to msiexec
	additionalArgs []string

	// cmdRunner allows injecting a custom command runner for testing
	cmdRunner cmdRunner

	// backoff allows injecting a custom backoff strategy for testing
	backoff backoff.BackOff
}

// MsiexecOption is an option type for creating msiexec command lines
type MsiexecOption func(*msiexecArgs) error

// Install specifies that msiexec will be invoked to install a product
func Install() MsiexecOption {
	return func(a *msiexecArgs) error {
		a.msiAction = "/i"
		return nil
	}
}

// AdministrativeInstall specifies that msiexec will be invoked to extract the product
func AdministrativeInstall() MsiexecOption {
	return func(a *msiexecArgs) error {
		a.msiAction = "/a"
		return nil
	}
}

// Uninstall specifies that msiexec will be invoked to uninstall a product
func Uninstall() MsiexecOption {
	return func(a *msiexecArgs) error {
		a.msiAction = "/x"
		return nil
	}
}

// WithMsi specifies the MSI target for msiexec
func WithMsi(target string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.target = target
		return nil
	}
}

// WithMsiFromPackagePath finds an MSI from the packages folder
func WithMsiFromPackagePath(target, product string) MsiexecOption {
	return func(a *msiexecArgs) error {
		updaterPath := filepath.Join(paths.PackagesPath, product, target)
		msis, err := filepath.Glob(filepath.Join(updaterPath, fmt.Sprintf("%s-*-1-x86_64.msi", product)))
		if err != nil {
			return err
		}
		if len(msis) > 1 {
			return fmt.Errorf("too many MSIs in package")
		} else if len(msis) == 0 {
			return fmt.Errorf("no MSIs in package")
		}
		a.target = msis[0]
		return nil
	}
}

// WithProduct specifies the product name to target for msiexec
func WithProduct(productName string) MsiexecOption {
	return func(a *msiexecArgs) error {
		product, err := FindProductCode(productName)
		if err != nil {
			return fmt.Errorf("error trying to find product %s: %w", productName, err)
		}
		a.target = product.Code
		return nil
	}
}

// WithLogFile specifies the log file for msiexec
func WithLogFile(logFile string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.logFile = logFile
		return nil
	}
}

// WithAdditionalArgs specifies additional arguments for msiexec
func WithAdditionalArgs(additionalArgs []string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.additionalArgs = append(a.additionalArgs, additionalArgs...)
		return nil
	}
}

// WithDdAgentUserName specifies the DDAGENTUSER_NAME to use
func WithDdAgentUserName(ddagentUserName string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.ddagentUserName = ddagentUserName
		return nil
	}
}

// WithDdAgentUserPassword specifies the DDAGENTUSER_PASSWORD to use
func WithDdAgentUserPassword(ddagentUserPassword string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.ddagentUserPassword = ddagentUserPassword
		return nil
	}
}

// HideControlPanelEntry passes a flag to msiexec so that the installed program
// does not show in the Control Panel "Add/Remove Software"
func HideControlPanelEntry() MsiexecOption {
	return func(a *msiexecArgs) error {
		a.additionalArgs = append(a.additionalArgs, "ARPSYSTEMCOMPONENT=1")
		return nil
	}
}

// withCmdRunner overrides how msiexec commands are executed.
//
// Note: intended only for testing.
func withCmdRunner(cmdRunner cmdRunner) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.cmdRunner = cmdRunner
		return nil
	}
}

// withBackOff overrides the default backoff strategy for msiexec retry logic
//
// Note: intended only for testing.
func withBackOff(backoffStrategy backoff.BackOff) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.backoff = backoffStrategy
		return nil
	}
}

// Msiexec is a type wrapping msiexec
type Msiexec struct {
	// logFile is the path to the MSI log file
	logFile string

	// postExecActions is a list of actions to be executed after msiexec has run
	postExecActions []func()

	// args saved for use in telemetry
	args *msiexecArgs

	// cmdRunner runs the execPath+cmdLine
	cmdRunner cmdRunner

	// backoff provides the retry strategy, for example for exit code 1618.
	// See isRetryableExitCode for more details.
	backoff backoff.BackOff

	// Command execution options
	execPath string
	cmdLine  string
}

func (m *Msiexec) openAndProcessLogFile() ([]byte, error) {
	logfile, err := os.Open(m.logFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File does not exist is not necessarily an error
			return nil, nil
		}
		return nil, err
	}
	result, err := m.processLogFile(logfile)
	_ = logfile.Close()
	return result, err
}

// processLogFile takes an open file and processes it with a series of processors to obtain
// a condensed version of the log file with only the relevant information.
func (m *Msiexec) processLogFile(logFile fs.File) ([]byte, error) {
	// Compile a list of regular expressions we are interested in extracting from the logs
	return processLogFile(logFile,
		func(bytes []byte) []TextRange {
			// Only need one TextRange of context before and after since other regexes will combine
			return FindAllIndexWithContext(regexp.MustCompile("Datadog[.]CustomActions.*"), bytes, 1, 1)
		},
		func(bytes []byte) []TextRange {
			// Only need one TextRange of context before and after since other regexes will combine
			return FindAllIndexWithContext(regexp.MustCompile("System[.]Exception"), bytes, 1, 1)
		},
		func(bytes []byte) []TextRange {
			// typically looks like this:
			// 	Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.StartDDServices
			// 	CA: 01:50:49: StartDDServices. Failed to start services: System.InvalidOperationException: Cannot start service datadogagent on computer '.'. ---> System.ComponentModel.Win32Exception: The service did not start due to a logon failure
			// 	  --- End of inner exception stack trace ---
			// 	  at System.ServiceProcess.ServiceController.Start(String args)
			// 	  at Datadog.CustomActions.Native.ServiceController.StartService(String serviceName, TimeSpan timeout)
			// 	  at Datadog.CustomActions.ServiceCustomAction.StartDDServices()
			// Other regexes will pick up on the stack trace, but there's not much information to get before the error
			return FindAllIndexWithContext(regexp.MustCompile("Cannot start service"), bytes, 1, 2)
		},
		func(bytes []byte) []TextRange {
			// Typically looks like this:
			// 	CA(ddnpm): DriverInstall:  serviceDef::create()
			// 	CA(ddnpm): DriverInstall:  Failed to CreateService 1073
			// 	CA(ddnpm): DriverInstall:  Service exists, verifying
			// 	CA(ddnpm): DriverInstall:  Updated path for existing service
			// So include a bit of context before and after
			return FindAllIndexWithContext(regexp.MustCompile("Failed to CreateService"), bytes, 5, 5)
		},
		func(bytes []byte) []TextRange {
			// Typically looks like this:
			//  Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.ProcessDdAgentUserCredentials
			//	CA: 01:49:43: LookupAccountWithExtendedDomainSyntax. User not found, trying again with fixed domain part: \toto
			//	CA: 01:49:43: ProcessDdAgentUserCredentials. User toto doesn't exist.
			//	CA: 01:49:43: ProcessDdAgentUserCredentials. domain part is empty, using default
			//	CA: 01:49:43: ProcessDdAgentUserCredentials. Installing with DDAGENTUSER_PROCESSED_NAME=toto and DDAGENTUSER_PROCESSED_DOMAIN=datadoghq-qa-labs.local
			//	CA: 01:49:43: HandleProcessDdAgentUserCredentialsException. Error processing ddAgentUser credentials: Datadog.CustomActions.InvalidAgentUserConfigurationException: A password was not provided. A password is a required when installing on Domain Controllers.
			//	   at Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentials(Boolean calledFromUIControl)
			//	MSI (s) (C8!50) [01:49:43:906]: Product: Datadog Agent -- A password was not provided. A password is a required when installing on Domain Controllers.
			//
			//	A password was not provided. A password is a required when installing on Domain Controllers.
			//	CustomAction ProcessDdAgentUserCredentials returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
			//	Action ended 1:49:43: ProcessDdAgentUserCredentials. Return value 3.
			// So include lots of context to ensure we get the full picture
			return FindAllIndexWithContext(regexp.MustCompile("A password was not provided"), bytes, 6, 6)
		},
		func(bytes []byte) []TextRange {
			// Typically looks like this:
			// 	Info 1603. The file C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe is being held in use by the following process: Name: process-agent, Id: 4704, Window Title: '(not determined yet)'. Close that application and retry.
			// Not much context to be had before and after
			return FindAllIndexWithContext(regexp.MustCompile("is being held in use by the following process"), bytes, 1, 1)
		},
		func(bytes []byte) []TextRange {
			// Typically looks like this:
			// 	Calling custom action AgentCustomActions!Datadog.AgentCustomActions.CustomActions.StartDDServices
			// 	CustomAction WixFailWhenDeferred returned actual error code 1603 (note this may not be 100% accurate if translation happened inside sandbox)
			// 	Action ended 2:11:49: InstallFinalize. Return value 3.
			// The important context is the TextRange after the error ("Return value 3") but the previous lines can include some useful information too
			return FindAllIndexWithContext(regexp.MustCompile("returned actual error"), bytes, 5, 1)
		},
		func(bytes []byte) []TextRange {
			// Typically looks like this:
			//   Action 12:24:00: InstallServices. Installing new services
			//   InstallServices: Service:
			//   Error 1923. Service 'Datadog Agent' (datadogagent) could not be installed. Verify that you have sufficient privileges to install system services.
			//   MSI (s) (54:EC) [12:25:53:886]: Product: Datadog Agent -- Error 1923. Service 'Datadog Agent' (datadogagent) could not be installed. Verify that you have sufficient privileges to install system services.
			return FindAllIndexWithContext(regexp.MustCompile("Verify that you have sufficient privileges to install system services"), bytes, 2, 1)
		})
}

// isRetryableExitCode returns true if the exit code indicates the msiexec operation should be retried
func isRetryableExitCode(err error) bool {
	if err == nil {
		return false
	}

	var exitError exitCodeError
	if errors.As(err, &exitError) {
		if exitError.ExitCode() == int(windows.ERROR_INSTALL_ALREADY_RUNNING) {
			return true
		}
	}

	return false
}

// Run runs msiexec synchronously with retry logic
func (m *Msiexec) Run(ctx context.Context) ([]byte, error) {
	var attemptCount int

	operation := func() (any, err error) {
		span, _ := telemetry.StartSpanFromContext(ctx, "msiexec")
		defer func() {
			// Add telemetry metadata about the msiexec operation
			// Don't artibrarily add MSI parameters to the span, as they may
			// contain sensitive information like DDAGENTUSER_PASSWORD.
			span.SetTag("params.action", m.args.msiAction)
			span.SetTag("params.target", m.args.target)
			span.SetTag("params.logfile", m.args.logFile)
			span.SetTag("attempt_count", attemptCount)
			if err != nil {
				span.SetTag("is_error_retryable", isRetryableExitCode(err))
			}
			span.Finish(err)
		}()

		attemptCount++

		// Execute the command
		err = m.cmdRunner.Run(m.execPath, m.cmdLine)

		// Return permanent error for non-retryable exit codes
		if err != nil && !isRetryableExitCode(err) {
			return nil, backoff.Permanent(err)
		}

		return nil, err
	}

	// Execute with retry
	_, err := backoff.Retry(ctx, operation,
		backoff.WithBackOff(m.backoff),
	)

	// Process log file once after all retries are complete
	logFileBytes, logErr := m.openAndProcessLogFile()
	if logErr != nil {
		err = errors.Join(err, logErr)
	}

	// Execute post-execution actions
	for _, p := range m.postExecActions {
		p()
	}

	return logFileBytes, err
}

// Cmd creates a new Msiexec wrapper around cmd.Exec that will call msiexec
func Cmd(options ...MsiexecOption) (*Msiexec, error) {
	a := &msiexecArgs{}
	for _, opt := range options {
		if err := opt(a); err != nil {
			return nil, err
		}
	}
	if a.msiAction == "" || a.target == "" {
		return nil, fmt.Errorf("argument error")
	}
	cmd := &Msiexec{
		args: a,
	}
	if len(a.logFile) == 0 {
		tempDir, err := os.MkdirTemp("", "datadog-installer-tmp")
		if err != nil {
			return nil, err
		}
		a.logFile = path.Join(tempDir, "msi.log")
		cmd.postExecActions = append(cmd.postExecActions, func() {
			_ = os.RemoveAll(tempDir)
		})
	}
	if a.ddagentUserName != "" {
		a.additionalArgs = append(a.additionalArgs, fmt.Sprintf("DDAGENTUSER_NAME=%s", a.ddagentUserName))
	}
	if a.ddagentUserPassword != "" {
		a.additionalArgs = append(a.additionalArgs, fmt.Sprintf("DDAGENTUSER_PASSWORD=%s", a.ddagentUserPassword))
	}
	if a.msiAction == "/i" {
		a.additionalArgs = append(a.additionalArgs, "MSIFASTINSTALL=7")
	}

	cmd.logFile = a.logFile

	// Create command line for the MSI execution after all options are processed
	// Do NOT pass the args to msiexec in exec.Command as it will apply some quoting algorithm (CommandLineToArgvW) that is
	// incompatible with msiexec. It will make arguments like `TARGETDIR` fail because they will be quoted.
	// Instead, we use the SysProcAttr.CmdLine option and do the quoting ourselves.
	args := append([]string{
		fmt.Sprintf(`"%s"`, msiexecPath),
		a.msiAction,
		fmt.Sprintf(`"%s"`, a.target),
		"/qn",
		"/log", fmt.Sprintf(`"%s"`, a.logFile),
	}, a.additionalArgs...)

	// Set command execution options
	// Don't call exec.Command("msiexec") to create the exec.Cmd struct
	// as it will try to lookup msiexec.exe using %PATH%.
	// Alternatively we could pass the full path of msiexec.exe to exec.Command(...)
	// but it's much simpler to create the struct manually.
	cmd.execPath = msiexecPath
	cmd.cmdLine = strings.Join(args, " ")

	// Set command runner (use provided one or default)
	if a.cmdRunner != nil {
		cmd.cmdRunner = a.cmdRunner
	} else {
		cmd.cmdRunner = newRealCmdRunner()
	}

	// Set backoff strategy (use provided one or default)
	if a.backoff != nil {
		cmd.backoff = a.backoff
	} else {
		b := backoff.NewExponentialBackOff()
		b.InitialInterval = 10 * time.Second
		b.MaxInterval = 120 * time.Second
		cmd.backoff = b
	}

	return cmd, nil
}
