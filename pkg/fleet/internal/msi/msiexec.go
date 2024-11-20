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
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"golang.org/x/text/encoding/unicode"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

type msiexecArgs struct {
	// target should be either a full path to a MSI, an URL to a MSI or a product code.
	target string

	// msiAction should be "/i" for installation, "/x" for uninstallation etc...
	msiAction string

	// logFile should be a full local path where msiexec will write the installation logs.
	// If nothing is specified, a random, temporary file is used.
	logFile         string
	ddagentUserName string
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

// WithDdAgentUserName specifies the DDAGENTUSER_NAME to use
func WithDdAgentUserName(ddagentUserName string) MsiexecOption {
	return func(a *msiexecArgs) error {
		a.ddagentUserName = ddagentUserName
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

func (m *Msiexec) readLogFile() ([]byte, error) {
	logFileBytes, err := os.ReadFile(m.logFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File does not exist is not necessarily an error
			return nil, nil
		}
		return nil, err
	}
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	return utf16.Bytes(logFileBytes)
}

// Run runs msiexec synchronously
func (m *Msiexec) Run() ([]byte, error) {
	err := m.Cmd.Run()
	// The log file *should not* be too big. Avoid verbose log files.
	logFileBytes, err2 := m.readLogFile()
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
		logFileBytes, err2 := m.readLogFile()
		err = errors.Join(err, err2)
		for _, p := range m.postExecActions {
			p()
		}
		done(logFileBytes, err)
	}()
	return nil
}

// FireAndForget starts msiexec and doesn't wait for it to finish.
// The log file won't be read at the end and not post execution actions will be executed.
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
		tempDir, err := os.MkdirTemp("", "datadog-installer")
		if err != nil {
			return nil, err
		}
		a.logFile = path.Join(tempDir, "msi.log")
		cmd.postExecActions = append(cmd.postExecActions, func() {
			_ = os.RemoveAll(tempDir)
		})
	}
	args := []string{a.msiAction, a.target, "/qn", "MSIFASTINSTALL=7", "/log", a.logFile}
	if a.ddagentUserName != "" {
		args = append(args, fmt.Sprintf("DDAGENTUSER_NAME=%s", a.ddagentUserName))
	}

	cmd.Cmd = exec.Command("msiexec", args...)
	cmd.logFile = a.logFile

	return cmd, nil
}
