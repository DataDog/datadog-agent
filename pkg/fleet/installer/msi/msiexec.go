// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package msi contains helper functions to work with msi packages
package msi

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

var (
	msiexecPath = `C:\Windows\System32\msiexec.exe`
)

func init() {
	system32, err := windows.KnownFolderPath(windows.FOLDERID_System, 0)
	if err == nil {
		msiexecPath = filepath.Join(system32, "msiexec.exe")
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

// Msiexec is a type wrapping msiexec
type Msiexec struct {
	*exec.Cmd

	// logFile is the path to the MSI log file
	logFile string

	// postExecActions is a list of actions to be executed after msiexec has run
	postExecActions []func()
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

// Run runs msiexec synchronously
func (m *Msiexec) Run() ([]byte, error) {
	err := m.Cmd.Run()
	// The log file *should not* be too big. Avoid verbose log files.
	logFileBytes, err2 := m.openAndProcessLogFile()
	err = errors.Join(err, err2)
	for _, p := range m.postExecActions {
		p()
	}

	return logFileBytes, err
}

// RunAsync runs msiexec asynchronously
func (m *Msiexec) RunAsync(done func([]byte, error)) error {
	err := m.Cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		err := m.Cmd.Wait()
		// The log file *should not* be too big. Avoid verbose log files.
		logFileBytes, err2 := m.openAndProcessLogFile()
		err = errors.Join(err, err2)
		for _, p := range m.postExecActions {
			p()
		}
		done(logFileBytes, err)
	}()
	return nil
}

// FireAndForget starts msiexec and doesn't wait for it to finish.
// The log file won't be read at the end and post execution actions will not be executed.
func (m *Msiexec) FireAndForget() error {
	return m.Cmd.Start()
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

	cmd := &Msiexec{}
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

	cmd.Cmd = &exec.Cmd{
		// Don't call exec.Command("msiexec") to create the exec.Cmd struct
		// as it will try to lookup msiexec.exe using %PATH%.
		// Alternatively we could pass the full path of msiexec.exe to exec.Command(...)
		// but it's much simpler to create the struct manually.
		Path: msiexecPath,
		SysProcAttr: &syscall.SysProcAttr{
			CmdLine: strings.Join(args, " "),
		},
	}
	cmd.logFile = a.logFile

	return cmd, nil
}
