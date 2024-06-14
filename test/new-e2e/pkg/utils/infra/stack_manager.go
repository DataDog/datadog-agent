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

	defaultStackUpTimeout      time.Duration = 60 * time.Minute
	defaultStackCancelTimeout  time.Duration = 10 * time.Minute
	defaultStackDestroyTimeout time.Duration = 60 * time.Minute
	stackDeleteTimeout         time.Duration = 20 * time.Minute
	stackUpMaxRetry                          = 2
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

func (i internalError) Is(target error) bool {
	_, ok := target.(internalError)
	return ok
}

// StackManager handles
type StackManager struct {
	stacks *safeStackMap

	knownErrors []knownError
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
		stacks:      newSafeStackMap(),
		knownErrors: getKnownErrors(),
	}, nil
}

// GetStack creates or return a stack based on stack name and config, if error occurs during stack creation it destroy all the resources created
func (sm *StackManager) GetStack(ctx context.Context, name string, config runner.ConfigMap, deployFunc pulumi.RunFunc, failOnMissing bool) (_ *auto.Stack, _ auto.UpResult, err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	stack, upResult, err := sm.getStack(
		ctx,
		name,
		deployFunc,
		WithConfigMap(config),
		WithFailOnMissing(failOnMissing),
	)
	if err != nil {
		errDestroy := sm.deleteStack(ctx, name, stack, nil, nil)
		if errDestroy != nil {
			return stack, upResult, errors.Join(err, errDestroy)
		}
	}

	return stack, upResult, err
}

type getStackParams struct {
	Config             runner.ConfigMap
	FailOnMissing      bool
	LogWriter          io.Writer
	DatadogEventSender datadogEventSender
	UpTimeout          time.Duration
	DestroyTimeout     time.Duration
	CancelTimeout      time.Duration
}

// GetStackOption is a function that sets a parameter for GetStack function
type GetStackOption func(*getStackParams)

// WithConfigMap sets the configuration map for the stack
func WithConfigMap(config runner.ConfigMap) GetStackOption {
	return func(p *getStackParams) {
		p.Config = config
	}
}

// WithFailOnMissing sets the failOnMissing flag for the stack
func WithFailOnMissing(failOnMissing bool) GetStackOption {
	return func(p *getStackParams) {
		p.FailOnMissing = failOnMissing
	}
}

// WithLogWriter sets the log writer for the stack
func WithLogWriter(logWriter io.Writer) GetStackOption {
	return func(p *getStackParams) {
		p.LogWriter = logWriter
	}
}

// WithDatadogEventSender sets the datadog event sender for the stack
func WithDatadogEventSender(datadogEventSender datadogEventSender) GetStackOption {
	return func(p *getStackParams) {
		p.DatadogEventSender = datadogEventSender
	}
}

// WithUpTimeout sets the up timeout for the stack
func WithUpTimeout(upTimeout time.Duration) GetStackOption {
	return func(p *getStackParams) {
		p.UpTimeout = upTimeout
	}
}

// WithDestroyTimeout sets the destroy timeout for the stack
func WithDestroyTimeout(destroyTimeout time.Duration) GetStackOption {
	return func(p *getStackParams) {
		p.DestroyTimeout = destroyTimeout
	}
}

// WithCancelTimeout sets the cancel timeout for the stack
func WithCancelTimeout(cancelTimeout time.Duration) GetStackOption {
	return func(p *getStackParams) {
		p.CancelTimeout = cancelTimeout
	}
}

// GetStackNoDeleteOnFailure creates or return a stack based on stack name and config, if error occurs during stack creation, it will not destroy the created resources. Using this can lead to resource leaks.
func (sm *StackManager) GetStackNoDeleteOnFailure(ctx context.Context, name string, deployFunc pulumi.RunFunc, options ...GetStackOption) (_ *auto.Stack, _ auto.UpResult, err error) {
	defer func() {
		if err != nil {
			err = internalError{err}
		}
	}()

	return sm.getStack(ctx, name, deployFunc, options...)
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

	return sm.deleteStack(ctx, name, stack, logWriter, nil)
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
		err := sm.deleteStack(ctx, stackID, stack, nil, nil)
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

func (sm *StackManager) getProgressStreamsOnUp(logger io.Writer) optup.Option {
	progressStreams, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.PulumiVerboseProgressStreams, false)
	if err != nil {
		return optup.ErrorProgressStreams(logger)
	}

	if progressStreams {
		return optup.ProgressStreams(logger)
	}

	return optup.ErrorProgressStreams(logger)
}

func (sm *StackManager) getProgressStreamsOnDestroy(logger io.Writer) optdestroy.Option {
	progressStreams, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.PulumiVerboseProgressStreams, false)
	if err != nil {
		return optdestroy.ErrorProgressStreams(logger)
	}

	if progressStreams {
		return optdestroy.ProgressStreams(logger)
	}
	return optdestroy.ErrorProgressStreams(logger)
}

func (sm *StackManager) deleteStack(ctx context.Context, stackID string, stack *auto.Stack, logWriter io.Writer, ddEventSender datadogEventSender) error {
	if stack == nil {
		return fmt.Errorf("unable to find stack, skipping deletion of: %s", stackID)
	}

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
	progressStreamsDestroyOption := sm.getProgressStreamsOnDestroy(logger)
	//initialize datadog event sender
	if ddEventSender == nil {
		ddEventSender = newDatadogEventSender(logger)
	}

	downCount := 0
	var destroyErr error
	for {
		downCount++
		destroyContext, cancel := context.WithTimeout(ctx, defaultStackDestroyTimeout)
		_, destroyErr = stack.Destroy(destroyContext, progressStreamsDestroyOption, optdestroy.DebugLogging(loggingOptions))
		cancel()
		if destroyErr == nil {
			sendEventToDatadog(ddEventSender, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack destroy", stackID), "", []string{"operation:destroy", "result:ok", fmt.Sprintf("stack:%s", stack.Name()), fmt.Sprintf("retries:%d", downCount)})
			break
		}

		// handle timeout
		contextCauseErr := context.Cause(destroyContext)
		if errors.Is(contextCauseErr, context.DeadlineExceeded) {
			sendEventToDatadog(ddEventSender, fmt.Sprintf("[E2E] Stack %s : timeout on Pulumi stack destroy", stackID), "", []string{"operation:destroy", fmt.Sprintf("stack:%s", stack.Name())})
			fmt.Fprint(logger, "Timeout during stack destroy, trying to cancel stack's operation\n")
			err := cancelStack(stack, defaultStackCancelTimeout)
			if err != nil {
				fmt.Fprintf(logger, "Giving up on error during attempt to cancel stack operation: %v\n", err)
				return err
			}
		}

		sendEventToDatadog(ddEventSender, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack destroy", stackID), destroyErr.Error(), []string{"operation:destroy", "result:fail", fmt.Sprintf("stack:%s", stack.Name()), fmt.Sprintf("retries:%d", downCount)})

		if downCount > stackUpMaxRetry {
			fmt.Printf("Giving up on error during stack destroy: %v\n", destroyErr)
			return destroyErr
		}
		fmt.Printf("Retrying stack on error during stack destroy: %v\n", destroyErr)
	}

	deleteContext, cancel := context.WithTimeout(ctx, stackDeleteTimeout)
	defer cancel()
	err = stack.Workspace().RemoveStack(deleteContext, stack.Name())
	return err
}

func (sm *StackManager) getStack(ctx context.Context, name string, deployFunc pulumi.RunFunc, options ...GetStackOption) (*auto.Stack, auto.UpResult, error) {
	params := getDefaultGetStackParams()
	for _, opt := range options {
		opt(&params)
	}

	// Build configuration from profile
	profile := runner.GetProfile()
	stackName := buildStackName(profile.NamePrefix(), name)
	deployFunc = runFuncWithRecover(deployFunc)

	// Inject common/managed parameters
	cm, err := runner.BuildStackParameters(profile, params.Config)
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
		if auto.IsSelectStack404Error(err) && !params.FailOnMissing {
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
	var logger = params.LogWriter

	progressStreamsUpOption := sm.getProgressStreamsOnUp(logger)
	progressStreamsDestroyOption := sm.getProgressStreamsOnDestroy(logger)

	var upResult auto.UpResult
	var upError error
	upCount := 0

	for {
		upCount++
		upCtx, cancel := context.WithTimeout(ctx, params.UpTimeout)
		now := time.Now()
		upResult, upError = stack.Up(upCtx, progressStreamsUpOption, optup.DebugLogging(loggingOptions))
		fmt.Fprintf(logger, "Stack up took %v\n", time.Since(now))
		cancel()

		// early return on success
		if upError == nil {
			sendEventToDatadog(params.DatadogEventSender, fmt.Sprintf("[E2E] Stack %s : success on Pulumi stack up", name), "", []string{"operation:up", "result:ok", fmt.Sprintf("stack:%s", stack.Name()), fmt.Sprintf("retries:%d", upCount)})
			break
		}

		// handle timeout
		contextCauseErr := context.Cause(upCtx)
		if errors.Is(contextCauseErr, context.DeadlineExceeded) {
			sendEventToDatadog(params.DatadogEventSender, fmt.Sprintf("[E2E] Stack %s : timeout on Pulumi stack up", name), "", []string{"operation:up", fmt.Sprintf("stack:%s", stack.Name())})
			fmt.Fprint(logger, "Timeout during stack up, trying to cancel stack's operation\n")
			err = cancelStack(stack, params.CancelTimeout)
			if err != nil {
				fmt.Fprintf(logger, "Giving up on error during attempt to cancel stack operation: %v\n", err)
				return stack, upResult, err
			}
		}

		retryStrategy := sm.getRetryStrategyFrom(upError, upCount)
		sendEventToDatadog(params.DatadogEventSender, fmt.Sprintf("[E2E] Stack %s : error on Pulumi stack up", name), upError.Error(), []string{"operation:up", "result:fail", fmt.Sprintf("retry:%s", retryStrategy), fmt.Sprintf("stack:%s", stack.Name()), fmt.Sprintf("retries:%d", upCount)})

		switch retryStrategy {
		case reUp:
			fmt.Fprintf(logger, "Retrying stack on error during stack up: %v\n", upError)
		case reCreate:
			fmt.Fprintf(logger, "Recreating stack on error during stack up: %v\n", upError)
			destroyCtx, cancel := context.WithTimeout(ctx, params.DestroyTimeout)
			_, err = stack.Destroy(destroyCtx, progressStreamsDestroyOption, optdestroy.DebugLogging(loggingOptions))
			cancel()
			if err != nil {
				fmt.Fprintf(logger, "Error during stack destroy at recrate stack attempt: %v\n", err)
				return stack, auto.UpResult{}, err
			}
		case noRetry:
			fmt.Fprintf(logger, "Giving up on error during stack up: %v\n", upError)
			return stack, upResult, upError
		}
	}

	return stack, upResult, upError
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

	fmt.Printf("Creating workspace for stack: %s at %s\n", stackName, workspaceStackDir)
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

func (sm *StackManager) getRetryStrategyFrom(err error, upCount int) retryType {
	// if first attempt + retries count are higher than max retry, give up
	if upCount > stackUpMaxRetry {
		return noRetry
	}

	for _, knownError := range sm.knownErrors {
		if strings.Contains(err.Error(), knownError.errorMessage) {
			return knownError.retryType
		}
	}

	return reUp
}

// sendEventToDatadog sends an event to Datadog, it will use the API Key from environment variable DD_API_KEY if present, otherwise it will use the one from SSM Parameter Store
func sendEventToDatadog(sender datadogEventSender, title string, message string, tags []string) {
	sender.SendEvent(datadogV1.EventCreateRequest{
		Title: title,
		Text:  message,
		Tags:  append([]string{"repository:datadog/datadog-agent", "test:new-e2e", "source:pulumi"}, tags...),
	})
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

func cancelStack(stack *auto.Stack, cancelTimeout time.Duration) error {
	if cancelTimeout.Nanoseconds() == 0 {
		cancelTimeout = defaultStackCancelTimeout
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	err := stack.Cancel(cancelCtx)
	cancel()

	if err == nil {
		return nil
	}

	// handle timeout
	ctxCauseErr := context.Cause(cancelCtx)
	if errors.Is(ctxCauseErr, context.DeadlineExceeded) {
		return fmt.Errorf("timeout during stack cancel: %w", ctxCauseErr)
	}

	return err
}

func getDefaultGetStackParams() getStackParams {
	var defaultLogger io.Writer = os.Stderr
	return getStackParams{
		Config:             nil,
		UpTimeout:          defaultStackUpTimeout,
		DestroyTimeout:     defaultStackDestroyTimeout,
		CancelTimeout:      defaultStackCancelTimeout,
		LogWriter:          defaultLogger,
		DatadogEventSender: newDatadogEventSender(defaultLogger),
		FailOnMissing:      false,
	}
}
