// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	deploymentIDFile = ".deployment-id"
	// incomingPrefix names the scratch directory an experiment is built in. It is created beside
	// the stable directory rather than under it, so publishing it is a rename within one parent
	// and never a copy across filesystems, and so the Agent never reads a half-built tree.
	incomingPrefix = ".datadog-config-incoming"
)

// experimentLink returns the resting link that owns the experiment configuration path.
func (d *Directories) experimentLink() restingLink {
	return restingLink{path: d.ExperimentPath, target: d.StablePath}
}

// GetState returns the state of the directories.
//
// The resting link is authoritative for whether an experiment is deployed: the deployment ID is
// only read from the experiment path once the link is known to stand as a real directory. Reading
// it first would report the stable deployment ID as an experiment's, because a resting link
// resolves every path under it to the stable directory.
func (d *Directories) GetState() (State, error) {
	stableDeploymentID, err := readDeploymentID(d.StablePath)
	if err != nil {
		return State{}, err
	}
	resting, err := d.experimentLink().IsResting()
	if err != nil {
		return State{}, err
	}
	if resting {
		return State{StableDeploymentID: stableDeploymentID}, nil
	}
	experimentDeploymentID, err := readDeploymentID(d.ExperimentPath)
	if err != nil {
		return State{}, err
	}
	return State{
		StableDeploymentID:     stableDeploymentID,
		ExperimentDeploymentID: experimentDeploymentID,
	}, nil
}

// WriteExperiment builds the experiment configuration directory and publishes it.
//
// The copy is made and patched in a scratch directory beside the stable one, and the resting link
// is replaced only once everything has applied cleanly. Nothing observable happens until that
// last rename, so a failure at any earlier point leaves a host that looks exactly like one where
// no experiment was ever started.
func (d *Directories) WriteExperiment(ctx context.Context, operations Operations) (err error) {
	link := d.experimentLink()
	resting, err := link.IsResting()
	if err != nil {
		return err
	}
	if !resting {
		return fmt.Errorf("a configuration experiment is already deployed at %s", d.ExperimentPath)
	}

	incoming, err := os.MkdirTemp(filepath.Dir(d.StablePath), incomingPrefix)
	if err != nil {
		return fmt.Errorf("could not create the scratch configuration directory: %w", err)
	}
	tree := configTree{sourcePath: d.StablePath, targetPath: incoming}
	defer func() {
		if err != nil {
			if discardErr := tree.Discard(ctx); discardErr != nil {
				log.Warnf("could not discard the scratch configuration directory: %v", discardErr)
			}
		}
	}()

	if err = tree.Copy(ctx); err != nil {
		return err
	}

	operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)
	if err = operations.Apply(ctx, incoming); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(incoming, deploymentIDFile), []byte(operations.DeploymentID), 0640); err != nil {
		return fmt.Errorf("could not write the deployment ID: %w", err)
	}
	return link.Materialize(incoming)
}

// PromoteExperiment makes the deployed experiment the stable configuration.
func (d *Directories) PromoteExperiment(ctx context.Context) error {
	link := d.experimentLink()
	resting, err := link.IsResting()
	if err != nil {
		return err
	}
	if resting {
		return fmt.Errorf("no configuration experiment is deployed at %s", d.ExperimentPath)
	}
	if err := (dirSwap{live: d.StablePath, incoming: d.ExperimentPath}).Commit(ctx); err != nil {
		return err
	}
	// The swap consumed the experiment path: it is now the stable directory. Put the link back,
	// so the host rests in the same shape it would have if no experiment had ever run.
	return link.Rest()
}

// RemoveExperiment discards the deployed experiment, if any.
func (d *Directories) RemoveExperiment(_ context.Context) error {
	return d.experimentLink().Rest()
}

func readDeploymentID(dir string) (string, error) {
	id, err := os.ReadFile(filepath.Join(dir, deploymentIDFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return string(id), nil
}

// setFileOwnershipAndPermissions sets the ownership and permissions for a file based on its
// configFileSpec. If the account doesn't exist (e.g. in tests) or the process doesn't have
// permission to change ownership, the function logs a warning and continues without failing.
func setFileOwnershipAndPermissions(ctx context.Context, root *os.Root, path string, spec *configFileSpec) error {
	if spec.mode != 0 {
		if err := root.Chmod(path, spec.mode); err != nil {
			return fmt.Errorf("error setting file permissions for %s: %w", path, err)
		}
	}
	if err := file.Chown(ctx, filepath.Join(root.Name(), path), spec.owner, spec.group); err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
	}
	return nil
}
