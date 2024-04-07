// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optremove"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	nameSep               = "-"
	e2eWorkspaceDirectory = "dd-e2e-workspace"

	stackUpTimeout      = 60 * time.Minute
	stackDestroyTimeout = 60 * time.Minute
	stackDeleteTimeout  = 20 * time.Minute
	stackUpRetry        = 2
)

var (
	defaultWorkspaceEnvVars = map[string]string{
		"PULUMI_SKIP_UPDATE_CHECK": "true",
	}

	stackManager     *StackManager
	initStackManager sync.Once
)

type internalError struct {
	err error
}

func (i internalError) Error() string {
	return fmt.Sprintf("E2E INTERNAL ERROR: %v", i.err)
}

// StackManager handles
type StackManager struct {
	stacks *safeStackMap

	retriableErrors []retriableError
}

type safeStackMap struct {
	stacks map[string]*auto.Stack
	lock   sync.RWMutex
}

func newSafeStackMap() *safeStackMap {
	return &safeStackMap{stacks: map[string]*auto.Stack{}, lock: sync.RWMutex{}}
}

func (s *safeStackMap) Get(key string) (*auto.Stack, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	stack, ok := s.stacks[key]
	return stack, ok
}

func (s *safeStackMap) Set(key string, value *auto.Stack) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.stacks[key] = value
}

func (s *safeStackMap) Range(f func(string, *auto.Stack)) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for key, value := range s.stacks {
		f(key, value)
	}
}

// GetStackManager returns a stack manager, initialising on first call
func GetStackManager() *StackManager {
	initStackManager.Do(func() {
		var err error

		stackManager, err = newStackManager()
		if err != nil {
			panic(fmt.Sprintf("Got an error during StackManager singleton init, err: %v", err))
		}
	})

	return stackManager
}

func newStackManager() (*StackManager, error) {
	return &StackManager{
		stacks:          newSafeStackMap(),
		retriableErrors: getKnownRetriableErrors(),
	}, nil
}

// GetStack creates or return a stack based on stack name and config, if error occurs during stack creation it destroy all the resources created
func (sm *StackManager) GetStack(ctx context.Context, name string, config runner.ConfigMap, deployFunc pulumi.RunFunc, failOnMissing bool) (_ *auto.Stack, _ auto.UpResult, err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	stack, upResult, err := sm.getStack(ctx, name, config, deployFunc, failOnMissing, nil)
	if err != nil {
		errDestroy := sm.deleteStack(ctx, name, stack, nil)
		if errDestroy != nil {
			return stack, upResult, errors.Join(err, errDestroy)
		}
	}

	return stack, upResult, err
}

// GetStackNoDeleteOnFailure creates or return a stack based on stack name and config, if error occurs during stack creation, it will not destroy the created resources. Using this can lead to resource leaks.
func (sm *StackManager) GetStackNoDeleteOnFailure(ctx context.Context, name string, config runner.ConfigMap, deployFunc pulumi.RunFunc, failOnMissing bool, logWriter io.Writer) (_ *auto.Stack, _ auto.UpResult, err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	return sm.getStack(ctx, name, config, deployFunc, failOnMissing, logWriter)
}

// DeleteStack safely deletes a stack
func (sm *StackManager) DeleteStack(ctx context.Context, name string, logWriter io.Writer) (err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	stack, ok := sm.stacks.Get(name)
	if !ok {
		// Build configuration from profile
		profile := runner.GetProfile()
		stackName := buildStackName(profile.NamePrefix(), name)
		workspace, err := buildWorkspace(ctx, profile, stackName, func(ctx *pulumi.Context) error { return nil })
		if err != nil {
			return err
		}

		newStack, err := auto.SelectStack(ctx, stackName, workspace)
		if err != nil {
			return err
		}

		stack = &newStack
	}

	return sm.deleteStack(ctx, name, stack, logWriter)
}

// ForceRemoveStackConfiguration removes the configuration files pulumi creates for managing a stack.
// It DOES NOT perform any cleanup of the resources created by the stack. Call `DeleteStack` for correct cleanup.
func (sm *StackManager) ForceRemoveStackConfiguration(ctx context.Context, name string) (err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	stack, ok := sm.stacks.Get(name)
	if !ok {
		return fmt.Errorf("unable to remove stack %s: stack not present", name)
	}

	deleteContext, cancel := context.WithTimeout(ctx, stackDeleteTimeout)
	defer cancel()
	return stack.Workspace().RemoveStack(deleteContext, stack.Name(), optremove.Force())
}

// Cleanup delete any existing stack
func (sm *StackManager) Cleanup(ctx context.Context) []error {
	var errors []error

	sm.stacks.Range(func(stackID string, stack *auto.Stack) {
		err := sm.deleteStack(ctx, stackID, stack, nil)
		if err != nil {
			errors = append(errors, internalError{err})
		}
	})

	return errors
}

func (sm *StackManager) getLoggingOptions() (debug.LoggingOptions, error) {
	logLevel, err := runner.GetProfile().ParamStore().GetIntWithDefault(parameters.PulumiLogLevel, 1)
	if err != nil {
		return debug.LoggingOptions{}, err
	}
	pulumiLogLevel := uint(logLevel)
	pulumiLogToStdErr, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.PulumiLogToStdErr, false)
	if err != nil {
		return debug.LoggingOptions{}, err
	}

	return debug.LoggingOptions{
		FlowToPlugins: true,
		LogLevel:      &pulumiLogLevel,
		LogToStdErr:   pulumiLogToStdErr,
	}, nil
}

func (sm *StackManager) deleteStack(ctx context.Context, stackID string, stack *auto.Stack, logWriter io.Writer) error {
	if stack == nil {
		return fmt.Errorf("unable to find stack, skipping deletion of: %s", stackID)
	}

	destroyContext, cancel := context.WithTimeout(ctx, stackDestroyTimeout)

	loggingOptions, err := sm.getLoggingOptions()
	if err != nil {
		return err
	}
	var logger io.Writer

	if logWriter == nil {
		logger = os.Stdout
	} else {
		logger = logWriter
	}
	_, err = stack.Destroy(destroyContext, optdestroy.ProgressStreams(logger), optdestroy.DebugLogging(loggingOptions))
	cancel()
	if err != nil {
		return err
	}

	deleteContext, cancel := context.WithTimeout(ctx, stackDeleteTimeout)
	defer cancel()
	err = stack.Workspace().RemoveStack(deleteContext, stack.Name())
	return err
}

func (sm *StackManager) getStack(ctx context.Context, name string, config runner.ConfigMap, deployFunc pulumi.RunFunc, failOnMissing bool, logWriter io.Writer) (*auto.Stack, auto.UpResult, error) {
	// Build configuration from profile
	profile := runner.GetProfile()
	stackName := buildStackName(profile.NamePrefix(), name)
	deployFunc = runFuncWithRecover(deployFunc)

	// Inject common/managed parameters
	cm, err := runner.BuildStackParameters(profile, config)
	if err != nil {
		return nil, auto.UpResult{}, err
	}
	stack, _ := sm.stacks.Get(name)
	if stack == nil {
		workspace, err := buildWorkspace(ctx, profile, stackName, deployFunc)
		if err != nil {
			return nil, auto.UpResult{}, err
		}

		newStack, err := auto.SelectStack(ctx, stackName, workspace)
		if auto.IsSelectStack404Error(err) && !failOnMissing {
			newStack, err = auto.NewStack(ctx, stackName, workspace)
		}
		if err != nil {
			return nil, auto.UpResult{}, err
		}

		stack = &newStack
		sm.stacks.Set(name, stack)
	} else {
		stack.Workspace().SetProgram(deployFunc)
	}

	err = stack.SetAllConfig(ctx, cm.ToPulumi())
	if err != nil {
		return nil, auto.UpResult{}, err
	}

	loggingOptions, err := sm.getLoggingOptions()
	if err != nil {
		return nil, auto.UpResult{}, err
	}
	var logger io.Writer

	if logWriter == nil {
		logger = os.Stderr
	} else {
		logger = logWriter
	}

	var upResult auto.UpResult

	for retry := 0; retry < stackUpRetry; retry++ {
		upCtx, cancel := context.WithTimeout(ctx, stackUpTimeout)
		upResult, err = stack.Up(upCtx, optup.ProgressStreams(logger), optup.DebugLogging(loggingOptions))
		cancel()

		if err == nil {
			break
		}
		if retryStrategy := sm.getRetryStrategyFrom(err); retryStrategy != noRetry {
			fmt.Fprintf(logger, "Got error that should be retried during stack up, retrying with %s strategy", retryStrategy)
			err := sendEventToDatadog(fmt.Sprintf("[E2E] Stack %s : retrying Pulumi stack up", name), err.Error(), []string{"operation:up", fmt.Sprintf("retry:%s", retryStrategy)}, logger)
			if err != nil {
				fmt.Fprintf(logger, "Got error when sending event to Datadog: %v", err)
			}

			if retryStrategy == reCreate {
				// If we are recreating the stack, we should destroy the stack first
				destroyCtx, cancel := context.WithTimeout(ctx, stackDestroyTimeout)
				_, err := stack.Destroy(destroyCtx, optdestroy.ProgressStreams(logger), optdestroy.DebugLogging(loggingOptions))
				cancel()
				if err != nil {
					return stack, auto.UpResult{}, err
				}
			}
		} else {
			break
		}
	}
	return stack, upResult, err
}

func buildWorkspace(ctx context.Context, profile runner.Profile, stackName string, runFunc pulumi.RunFunc) (auto.Workspace, error) {
	project := workspace.Project{
		Name:           tokens.PackageName(profile.ProjectName()),
		Runtime:        workspace.NewProjectRuntimeInfo("go", nil),
		Description:    pulumi.StringRef("E2E Test inline project"),
		StackConfigDir: stackName,
		Config: map[string]workspace.ProjectConfigType{
			// Always disable
			"pulumi:disable-default-providers": {
				Value: []string{"*"},
			},
		},
	}

	// create workspace directory
	workspaceStackDir := profile.GetWorkspacePath(stackName)
	if err := os.MkdirAll(workspaceStackDir, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create temporary folder at: %s, err: %w", workspaceStackDir, err)
	}

	fmt.Printf("Creating workspace for stack: %s at %s", stackName, workspaceStackDir)
	return auto.NewLocalWorkspace(ctx,
		auto.Project(project),
		auto.Program(runFunc),
		auto.WorkDir(workspaceStackDir),
		auto.EnvVars(defaultWorkspaceEnvVars),
	)
}

func buildStackName(namePrefix, stackName string) string {
	stackName = namePrefix + nameSep + stackName
	return strings.ToLower(strings.ReplaceAll(stackName, "_", "-"))
}

func runFuncWithRecover(f pulumi.RunFunc) pulumi.RunFunc {
	return func(ctx *pulumi.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stackDump := make([]byte, 4096)
				stackSize := runtime.Stack(stackDump, false)
				err = fmt.Errorf("panic in run function, stack:\n %s\n\nerror: %v", stackDump[:stackSize], r)
			}
		}()

		return f(ctx)
	}
}

func (sm *StackManager) getRetryStrategyFrom(err error) retryType {
	for _, retriableError := range sm.retriableErrors {
		if strings.Contains(err.Error(), retriableError.errorMessage) {
			return retriableError.retryType
		}
	}
	return noRetry
}

// sendEventToDatadog sends an event to Datadog, it will use the API Key from environment variable DD_API_KEY if present, otherwise it will use the one from SSM Parameter Store
func sendEventToDatadog(title string, message string, tags []string, logger io.Writer) error {
	apiKey, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.APIKey, "")
	if err != nil {
		fmt.Fprintf(logger, "error when getting API key from parameter store: %v", err)
		return err
	}

	if apiKey == "" {
		fmt.Fprintf(logger, "Skipping sending event because API key is empty")
		return nil
	}

	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {
			Key: apiKey,
		},
	})

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV1.NewEventsApi(apiClient)

	_, r, err := api.CreateEvent(ctx, datadogV1.EventCreateRequest{
		Title: title,
		Text:  message,
		Tags:  append([]string{"repository:datadog/datadog-agent", "test:new-e2e", "source:pulumi"}, tags...),
	})
	if err != nil {
		fmt.Fprintf(logger, "error when calling `EventsApi.CreateEvent`: %v", err)
		fmt.Fprintf(logger, "Full HTTP response: %v\n", r)
		return err
	}
	return nil
}

// GetPulumiStackName returns the Pulumi stack name
// The internal Pulumi stack name should normally remain hidden as all the Pulumi interactions
// should be done via the StackManager.
// The only use case for getting the internal Pulumi stack name is to interact directly with Pulumi for debug purposes.
func (sm *StackManager) GetPulumiStackName(name string) (_ string, err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	stack, ok := sm.stacks.Get(name)
	if !ok {
		return "", fmt.Errorf("stack %s not present", name)
	}

	return stack.Name(), nil
}
