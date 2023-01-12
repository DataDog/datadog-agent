// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	projectName          = "ddagent-e2e"
	stackPrefix          = "ddagent-e2e"
	environmentParamName = "ddinfra:env"

	stackUpTimeout      = 20 * time.Minute
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
func (sm *StackManager) GetStack(ctx context.Context, envName string, name string, config auto.ConfigMap, deployFunc pulumi.RunFunc) (auto.UpResult, error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	// Set environment
	if config == nil {
		config = auto.ConfigMap{}
	}
	config[environmentParamName] = auto.ConfigValue{Value: envName}

	finalStackName := stackName(name)
	stackID := stackID(envName, name)
	stack := sm.stacks[stackID]
	if stack == nil {
		newStack, err := auto.UpsertStackInlineSource(ctx, finalStackName, projectName, deployFunc)
		if err != nil {
			return auto.UpResult{}, err
		}
		stack = &newStack
		sm.stacks[stackID] = stack
	}

	err := stack.SetAllConfig(ctx, config)
	if err != nil {
		return auto.UpResult{}, err
	}

	upCtx, cancel := context.WithTimeout(ctx, stackUpTimeout)
	defer cancel()
	return stack.Up(upCtx, optup.ProgressStreams(os.Stdout))
}

func (sm *StackManager) DeleteStack(ctx context.Context, envName string, name string) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	stackID := stackID(envName, name)
	return sm.deleteStack(ctx, stackID, sm.stacks[stackID])
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

func stackName(stackName string) string {
	var username string
	user, err := user.Current()
	if err == nil {
		username = user.Username
	}

	if username == "" || username == "root" {
		username = "nouser"
	}
	username = strings.ToLower(username)
	username = strings.ReplaceAll(username, ".", "-")
	username = strings.ReplaceAll(username, " ", "-")

	return stackPrefix + "-" + username + "-" + stackName
}

func stackID(envName string, stackName string) string {
	return envName + "/" + stackName
}
