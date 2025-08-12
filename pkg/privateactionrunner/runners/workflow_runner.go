package runners

import (
	"context"
	"fmt"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/helpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	privatebundles "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type WorkflowRunner struct {
	registry     *privatebundles.Registry
	opmsClient   opms.Client
	resolver     credentials.PrivateCredentialResolver
	config       *config.Config
	taskVerifier *taskverifier.TaskVerifier
	keysManager  remoteconfig.KeysManager
	taskLoop     *Loop
	log          log.Component
}

func NewWorkflowRunner(configuration *config.Config, log log.Component, keysManager remoteconfig.KeysManager, verifier *taskverifier.TaskVerifier, opmsClient opms.Client) *WorkflowRunner {

	return &WorkflowRunner{
		registry:     privatebundles.NewRegistry(configuration),
		opmsClient:   opmsClient,
		resolver:     credentials.NewPrivateCredentialResolver(),
		config:       configuration,
		keysManager:  keysManager,
		taskVerifier: verifier,
		log:          log,
	}
}

func (n *WorkflowRunner) Start(ctx context.Context) {
	if n.taskLoop != nil {
		n.log.Warn("WorkflowRunner already started")
		return
	}
	n.taskLoop = NewLoop(n, n.log)
	go func() {
		n.log.Info("Waiting for KeysManager to be ready")
		n.keysManager.WaitForReady()
		n.taskLoop.Run(ctx)
	}()
}

func (n *WorkflowRunner) Close(ctx context.Context) {
	if n.taskLoop != nil {
		n.taskLoop.Close(ctx)
	}
}

func (n *WorkflowRunner) RunTask(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	fqn := task.GetFQN()
	bundleName, actionName := helpers.SplitFQN(fqn)
	bundle := n.registry.GetBundle(bundleName)
	if bundle == nil {
		return nil, helpers.NewPARError(
			errorcode.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find bundle for %s", bundleName),
		)
	}
	action := bundle.GetAction(actionName)
	if action == nil {
		return nil, helpers.NewPARError(
			errorcode.ActionPlatformErrorCode_INTERNAL_ERROR,
			fmt.Errorf("could not find action for %s", actionName),
		)
	}
	// TODO check action allowlist and URL allowlist for http

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go n.startHeartbeat(heartbeatCtx, task)

	output, err := action.Run(ctx, task, credential)

	if err != nil {
		return nil, helpers.DefaultActionError(err)
	}

	return output, nil
}

func (n *WorkflowRunner) startHeartbeat(ctx context.Context, task *types.Task) {
	ticker := time.NewTicker(n.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.log.Infof("Heartbeat stopped for task %s", task.Data.ID)
			return
		case <-ticker.C:
			err := n.opmsClient.Heartbeat(ctx, task.Data.ID, task.GetFQN(), task.Data.Attributes.JobId)

			if err != nil {
				n.log.Errorf("Failed to send heartbeat %v", err)
			} else {
				n.log.Info("Heartbeat sent successfully")
			}
		}
	}
}
