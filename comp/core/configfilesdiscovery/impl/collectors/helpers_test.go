// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"errors"
	"testing"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfigFile(t *testing.T) {
	commandlineErr := errors.New("command line unavailable")
	readErr := errors.New("read failed")
	defaultPaths := []string{"/default/one.conf", "/default/two.conf"}

	tests := []struct {
		name                    string
		commandline             configfilesdiscoveryimpl.TargetCommandline
		commandlineErr          error
		commandlines            []configfilesdiscoveryimpl.TargetCommandline
		files                   map[string]configfilesdiscoveryimpl.ConfigFile
		readErrors              map[string]error
		defaultPathGroups       [][]string
		cancelContext           bool
		wantSelection           *configFileSelection
		wantErr                 error
		wantReadFileCalls       []string
		wantProcessCommandCalls int
	}{
		{
			name: "runtime explicit path wins",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args:       []string{"service", "/runtime.conf"},
				WorkingDir: "/runtime",
			},
			commandlines: []configfilesdiscoveryimpl.TargetCommandline{{
				Args: []string{"service", "/process.conf"},
			}},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/runtime.conf": {Path: "/runtime.conf"},
			},
			wantSelection: &configFileSelection{
				file:       configfilesdiscoveryimpl.ConfigFile{Path: "/runtime.conf"},
				workingDir: "/runtime",
			},
			wantReadFileCalls: []string{"/runtime.conf"},
		},
		{
			name: "explicit read error does not fall back",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"service", "/runtime.conf"},
			},
			readErrors: map[string]error{
				"/runtime.conf": readErr,
			},
			wantErr:           readErr,
			wantReadFileCalls: []string{"/runtime.conf"},
		},
		{
			name: "unresolved explicit path does not fall back",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"service", "relative.conf"},
			},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/one.conf": {Path: "/default/one.conf"},
			},
			wantProcessCommandCalls: 1,
		},
		{
			name: "recognized command without explicit path does not fall back",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"service"},
			},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/one.conf": {Path: "/default/one.conf"},
			},
			wantProcessCommandCalls: 1,
		},
		{
			name:           "recognized live command without explicit path recovers command line error",
			commandlineErr: commandlineErr,
			commandlines: []configfilesdiscoveryimpl.TargetCommandline{{
				Args: []string{"service"},
			}},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/one.conf": {Path: "/default/one.conf"},
			},
			wantProcessCommandCalls: 1,
		},
		{
			name: "live path resolves unresolved runtime path",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"service", "relative.conf"},
			},
			commandlines: []configfilesdiscoveryimpl.TargetCommandline{{
				Args: []string{"service", "/process.conf"},
			}},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/process.conf": {Path: "/process.conf"},
			},
			wantSelection:           &configFileSelection{file: configfilesdiscoveryimpl.ConfigFile{Path: "/process.conf"}},
			wantReadFileCalls:       []string{"/process.conf"},
			wantProcessCommandCalls: 1,
		},
		{
			name: "first live config keeps its process working directory",
			commandline: configfilesdiscoveryimpl.TargetCommandline{
				Args: []string{"wrapper"},
			},
			commandlines: []configfilesdiscoveryimpl.TargetCommandline{
				{Args: []string{"service", "/one.conf"}, WorkingDir: "/one"},
				{Args: []string{"service", "/two.conf"}, WorkingDir: "/two"},
			},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/one.conf": {Path: "/one.conf"},
				"/two.conf": {Path: "/two.conf"},
			},
			wantSelection: &configFileSelection{
				file:       configfilesdiscoveryimpl.ConfigFile{Path: "/one.conf"},
				workingDir: "/one",
			},
			wantReadFileCalls:       []string{"/one.conf"},
			wantProcessCommandCalls: 1,
		},
		{
			name: "higher priority default group wins",
			defaultPathGroups: [][]string{
				{"/preferred/config.conf"},
				defaultPaths,
			},
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/preferred/config.conf": {Path: "/preferred/config.conf"},
				"/default/one.conf":      {Path: "/default/one.conf"},
			},
			wantSelection:           &configFileSelection{file: configfilesdiscoveryimpl.ConfigFile{Path: "/preferred/config.conf"}},
			wantReadFileCalls:       []string{"/preferred/config.conf"},
			wantProcessCommandCalls: 1,
		},
		{
			name:           "unique default path recovers command line error",
			commandlineErr: commandlineErr,
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/two.conf": {Path: "/default/two.conf", Content: []byte("config")},
			},
			wantSelection: &configFileSelection{
				file: configfilesdiscoveryimpl.ConfigFile{
					Path:    "/default/two.conf",
					Content: []byte("config"),
				},
			},
			wantReadFileCalls:       defaultPaths,
			wantProcessCommandCalls: 1,
		},
		{
			name:                    "missing default paths preserve command line error",
			commandlineErr:          commandlineErr,
			wantErr:                 commandlineErr,
			wantReadFileCalls:       defaultPaths,
			wantProcessCommandCalls: 1,
		},
		{
			name: "multiple default paths are ambiguous",
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/one.conf": {Path: "/default/one.conf"},
				"/default/two.conf": {Path: "/default/two.conf"},
			},
			wantReadFileCalls:       defaultPaths,
			wantProcessCommandCalls: 1,
		},
		{
			name:           "multiple default paths preserve command line error",
			commandlineErr: commandlineErr,
			files: map[string]configfilesdiscoveryimpl.ConfigFile{
				"/default/one.conf": {Path: "/default/one.conf"},
				"/default/two.conf": {Path: "/default/two.conf"},
			},
			wantErr:                 commandlineErr,
			wantReadFileCalls:       defaultPaths,
			wantProcessCommandCalls: 1,
		},
		{
			name:                    "context cancellation stops default probing",
			cancelContext:           true,
			wantErr:                 context.Canceled,
			wantReadFileCalls:       []string{"/default/one.conf"},
			wantProcessCommandCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			reader := &configFileTestReader{
				commandline:    tt.commandline,
				commandlineErr: tt.commandlineErr,
				commandlines:   tt.commandlines,
				files:          tt.files,
				readErrors:     tt.readErrors,
			}

			defaultPathGroups := tt.defaultPathGroups
			if defaultPathGroups == nil {
				defaultPathGroups = [][]string{defaultPaths}
			}
			selection, err := selectConfigFile(ctx, reader, getTestConfigArg, matchesTestCommandline, "", defaultPathGroups...)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantSelection == nil {
				assert.Nil(t, selection)
			} else {
				require.NotNil(t, selection)
				assert.Equal(t, tt.wantSelection.file, selection.file)
				assert.Equal(t, tt.wantSelection.file.Path, selection.path.String())
				assert.Equal(t, tt.wantSelection.workingDir, selection.workingDir)
			}
			assert.Equal(t, tt.wantReadFileCalls, reader.readFileCalls)
			assert.Equal(t, tt.wantProcessCommandCalls, reader.processCommandlineCalls)
		})
	}
}

func getTestConfigArg(args []string) (string, bool) {
	if len(args) != 2 || args[0] != "service" {
		return "", false
	}
	return args[1], true
}

// verifyTestConfigFilePath returns a verified path or fails the current test.
func verifyTestConfigFilePath(t testing.TB, value string) configfilesdiscoveryimpl.VerifiedConfigFilePath {
	t.Helper()
	verified, err := configfilesdiscoveryimpl.VerifyConfigFilePath(configfilesdiscoveryimpl.UnverifiedConfigFilePath(value))
	require.NoError(t, err)
	return verified
}

// verifyTestConfigFilePattern returns a verified pattern or fails the current test.
func verifyTestConfigFilePattern(t testing.TB, value string) configfilesdiscoveryimpl.VerifiedConfigFilePattern {
	t.Helper()
	verified, err := configfilesdiscoveryimpl.VerifyConfigFilePattern(configfilesdiscoveryimpl.UnverifiedConfigFilePattern(value))
	require.NoError(t, err)
	return verified
}

func matchesTestCommandline(args []string) bool {
	return len(args) > 0 && args[0] == "service"
}

type configFileTestReader struct {
	commandline             configfilesdiscoveryimpl.TargetCommandline
	commandlineErr          error
	commandlines            []configfilesdiscoveryimpl.TargetCommandline
	files                   map[string]configfilesdiscoveryimpl.ConfigFile
	readErrors              map[string]error
	readFileCalls           []string
	processCommandlineCalls int
}

func (r *configFileTestReader) Runtime() configfilesdiscoveryimpl.RuntimeType {
	return configfilesdiscoveryimpl.RuntimeDocker
}

func (r *configFileTestReader) Close() {}

func (r *configFileTestReader) ReadFile(ctx context.Context, path configfilesdiscoveryimpl.VerifiedConfigFilePath) (configfilesdiscoveryimpl.ConfigFile, error) {
	r.readFileCalls = append(r.readFileCalls, path.String())
	if err := ctx.Err(); err != nil {
		return configfilesdiscoveryimpl.ConfigFile{}, err
	}
	if err, found := r.readErrors[path.String()]; found {
		return configfilesdiscoveryimpl.ConfigFile{}, err
	}
	if file, found := r.files[path.String()]; found {
		return file, nil
	}
	return configfilesdiscoveryimpl.ConfigFile{}, errors.New("file not found")
}

// FindFiles is not implemented by this test reader.
func (r *configFileTestReader) FindFiles(context.Context, configfilesdiscoveryimpl.VerifiedConfigFilePattern, int, configfilesdiscoveryimpl.ConfigFilePathMatcher) ([]configfilesdiscoveryimpl.VerifiedConfigFilePath, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (r *configFileTestReader) ReadEnvVars(context.Context, configfilesdiscoveryimpl.ConfigEnvVarPredicate) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r *configFileTestReader) ReadRuntimeCommandline(context.Context) (configfilesdiscoveryimpl.TargetCommandline, error) {
	if r.commandlineErr != nil {
		return configfilesdiscoveryimpl.TargetCommandline{}, r.commandlineErr
	}
	return r.commandline, nil
}

func (r *configFileTestReader) ReadLiveProcessCommandlines(context.Context) []configfilesdiscoveryimpl.TargetCommandline {
	r.processCommandlineCalls++
	return r.commandlines
}
