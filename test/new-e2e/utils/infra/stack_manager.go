// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/runner"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	projectName = "dd-e2e"
	nameSep     = "-"

	stackUpTimeout      = 60 * time.Minute
	stackDestroyTimeout = 60 * time.Minute
	stackDeleteTimeout  = 20 * time.Minute
)

var (
	stackManager     *StackManager
	initStackManager sync.Once
)

// StackManager handles
type StackManager struct {
	stacks map[string]*auto.Stack
	lock   sync.RWMutex
}

func GetStackManager() *StackManager {
	initStackManager.Do(func() {
		var err error

		stackManager, err = newStackManager(context.Background())
		if err != nil {
			panic(fmt.Sprintf("Got an error during StackManager singleton init, err: %v", err))
		}
	})

	return stackManager
}

func newStackManager(ctx context.Context) (*StackManager, error) {
	return &StackManager{
		stacks: make(map[string]*auto.Stack),
	}, nil
}

// GetStack creates or return a stack based on env+stack name
func (sm *StackManager) GetStack(ctx context.Context, name string, config runner.ConfigMap, deployFunc pulumi.RunFunc, failOnMissing bool) (*auto.Stack, auto.UpResult, error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	// Build configuration from profile
	profile := runner.GetProfile()
	stackName := buildStackName(profile.NamePrefix(), name)
	deployFunc = runFuncWithRecover(deployFunc)

	// Inject common/managed parameters
	cm, err := runner.BuildStackParameters(profile, config)
	if err != nil {
		return nil, auto.UpResult{}, err
	}

	stack := sm.stacks[name]
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
		sm.stacks[name] = stack
	}

	err = stack.SetAllConfig(ctx, cm.ToPulumi())
	if err != nil {
		return nil, auto.UpResult{}, err
	}

	upCtx, cancel := context.WithTimeout(ctx, stackUpTimeout)
	var loglevel uint = 1
	defer cancel()
	upResult, err := stack.Up(upCtx, optup.ProgressStreams(os.Stderr), optup.DebugLogging(debug.LoggingOptions{
		LogToStdErr:   true,
		FlowToPlugins: true,
		LogLevel:      &loglevel,
	}))
	return stack, upResult, err
}

func (sm *StackManager) DeleteStack(ctx context.Context, name string) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	return sm.deleteStack(ctx, name, sm.stacks[name])
}

func (sm *StackManager) Cleanup(ctx context.Context) []error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	var errors []error

	for stackID, stack := range sm.stacks {
		err := sm.deleteStack(ctx, stackID, stack)
		if err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func (sm *StackManager) deleteStack(ctx context.Context, stackID string, stack *auto.Stack) error {
	if stack == nil {
		return fmt.Errorf("unable to find stack, skipping deletion of: %s", stackID)
	}

	_, err := stack.Refresh(ctx)
	if err != nil {
		return err
	}

	destroyContext, cancel := context.WithTimeout(ctx, stackDestroyTimeout)
	_, err = stack.Destroy(destroyContext, optdestroy.ProgressStreams(os.Stdout))
	cancel()
	if err != nil {
		return err
	}

	deleteContext, cancel := context.WithTimeout(ctx, stackDeleteTimeout)
	defer cancel()
	err = stack.Workspace().RemoveStack(deleteContext, stack.Name())
	return err
}

func buildWorkspace(ctx context.Context, profile runner.Profile, stackName string, runFunc pulumi.RunFunc) (auto.Workspace, error) {
	project := workspace.Project{
		Name:           tokens.PackageName(projectName),
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

	return auto.NewLocalWorkspace(ctx, auto.Project(project), auto.Program(runFunc), auto.WorkDir(profile.RootWorkspacePath()))
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
