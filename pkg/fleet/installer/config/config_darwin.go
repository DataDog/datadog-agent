// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// incomingPrefix names the scratch directory an experiment is built in. It is created beside
	// the stable directory rather than under it, so publishing it is a rename within one parent
	// and never a copy across filesystems, and so the Agent never reads a half-built tree.
	incomingPrefix = ".datadog-config-incoming"
	// StateFileName is the file, in the root-owned state directory, that records which
	// configuration is deployed.
	StateFileName = "config-state.json"
	// maxStateSize bounds how much of the state file is read.
	maxStateSize = 4096
)

// defaultStatePath is where the state file lives when Directories.StatePath is empty.
var defaultStatePath = filepath.Join(paths.ConfigExperimentStatePath, StateFileName)

// configState is what the state file records.
//
// It lives outside the configuration tree, which belongs to the Agent's account, so only root can
// change it. The experiment is recorded together with the inode of its directory: a rename keeps a
// directory's inode, and only root can place anything at the stable or experiment path, so
// comparing the two paths' inodes against the record tells, after a crash at any point, whether
// the experiment was published, promoted, or neither. Only the inode is kept: both paths are on
// one volume, and a volume's device number can change across a reboot.
type configState struct {
	StableDeploymentID string            `json:"stable_deployment_id,omitempty"`
	Experiment         *experimentRecord `json:"experiment,omitempty"`
}

// experimentRecord is the configuration experiment last published or about to be.
type experimentRecord struct {
	DeploymentID string `json:"deployment_id"`
	Inode        uint64 `json:"inode"`
}

func (d *Directories) statePath() string {
	if d.StatePath != "" {
		return d.StatePath
	}
	return defaultStatePath
}

// experimentLink returns the resting link that owns the experiment configuration path.
func (d *Directories) experimentLink() restingLink {
	return restingLink{path: d.ExperimentPath, target: d.StablePath}
}

// GetState returns the state of the directories.
func (d *Directories) GetState() (State, error) {
	_, settled, deployed, err := d.resolve()
	if err != nil {
		return State{}, err
	}
	state := State{StableDeploymentID: settled.StableDeploymentID}
	if deployed {
		state.ExperimentDeploymentID = settled.Experiment.DeploymentID
	}
	return state, nil
}

// resolve reads the state file and matches its record against the directories on disk. It
// returns the file as read, the state the directories actually settle on, and whether an
// experiment is deployed:
//
//   - the stable path holds the recorded directory: a promote swapped it into place and stopped
//     before recording that, so the experiment is the stable configuration now;
//   - the experiment path is a real directory holding it: the experiment is deployed;
//   - neither: it was never published, or was discarded, and the record is stale.
func (d *Directories) resolve() (recorded configState, settled configState, deployed bool, err error) {
	recorded, err = d.readState()
	if err != nil || recorded.Experiment == nil {
		return recorded, recorded, false, err
	}
	stable, ok, err := inodeOf(d.StablePath)
	if err != nil {
		return recorded, configState{}, false, err
	}
	if ok && stable == recorded.Experiment.Inode {
		return recorded, configState{StableDeploymentID: recorded.Experiment.DeploymentID}, false, nil
	}
	experiment, ok, err := inodeOf(d.ExperimentPath)
	if err != nil {
		return recorded, configState{}, false, err
	}
	if ok && experiment == recorded.Experiment.Inode {
		return recorded, recorded, true, nil
	}
	return recorded, configState{StableDeploymentID: recorded.StableDeploymentID}, false, nil
}

// inodeOf returns the inode of the directory at path, without following a link. ok is false when
// nothing, or something other than a directory, is there.
func inodeOf(path string) (inode uint64, ok bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("could not inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return 0, false, nil
	}
	st, isStat := info.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, false, fmt.Errorf("could not read the inode of %s", path)
	}
	return st.Ino, true, nil
}

// WriteExperiment builds the experiment configuration directory and publishes it.
//
// The copy is made and patched in a scratch directory beside the stable one, recorded in the state
// file, and only then published by a rename. Nothing observable happens until that rename, so a
// failure at any earlier point leaves a host that looks exactly like one where no experiment was
// ever started.
func (d *Directories) WriteExperiment(ctx context.Context, operations Operations) (err error) {
	_, settled, deployed, err := d.resolve()
	if err != nil {
		return err
	}
	if deployed {
		return fmt.Errorf("a configuration experiment is already deployed at %s", d.ExperimentPath)
	}
	link := d.experimentLink()
	// Anything else at the experiment path is not a recorded experiment -- the previous stable
	// configuration left by an interrupted promote, or a stray directory -- so it is cleared.
	if err = link.Rest(); err != nil {
		return err
	}

	// A prior run may have crashed before publishing its scratch directory. With the link resting,
	// any such leftover is safe to discard.
	discardStaleScratch(filepath.Dir(d.StablePath))

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
	// Unlike Linux and Windows, no legacy managed/ layout is migrated here: macOS never had one, and
	// that migration walks and reads the stable tree, which belongs to the Agent's account, as
	// root while following its links.
	if err = operations.Apply(ctx, incoming); err != nil {
		return err
	}
	if err = tree.Finish(ctx); err != nil {
		return err
	}
	inode, ok, err := inodeOf(incoming)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not a directory", incoming)
	}
	// Recorded before the publish: a crash in between leaves a record no directory matches, which
	// resolve ignores. The stable ID is the settled one, so an interrupted promote is completed.
	record := configState{
		StableDeploymentID: settled.StableDeploymentID,
		Experiment:         &experimentRecord{DeploymentID: operations.DeploymentID, Inode: inode},
	}
	if err = d.writeState(record); err != nil {
		return fmt.Errorf("could not record the configuration experiment: %w", err)
	}
	if err = link.Materialize(incoming); err != nil {
		if clearErr := d.writeState(settled); clearErr != nil {
			log.Warnf("could not clear the record of the unpublished experiment: %v", clearErr)
		}
		return err
	}
	return nil
}

// PromoteExperiment makes the deployed experiment the stable configuration.
func (d *Directories) PromoteExperiment(ctx context.Context) error {
	_, settled, deployed, err := d.resolve()
	if err != nil {
		return err
	}
	if !deployed {
		return fmt.Errorf("no configuration experiment is deployed at %s", d.ExperimentPath)
	}
	if err := (dirSwap{live: d.StablePath, incoming: d.ExperimentPath}).Commit(ctx); err != nil {
		return err
	}
	// The swap is the promote: from here resolve finds the recorded directory at the stable path,
	// whether or not the lines below run.
	if err := d.writeState(configState{StableDeploymentID: settled.Experiment.DeploymentID}); err != nil {
		return fmt.Errorf("could not record the promoted configuration: %w", err)
	}
	return d.experimentLink().Rest()
}

// RemoveExperiment discards the deployed experiment, if any, and settles the state file on what
// the directories hold, which also finishes a promote interrupted after its swap.
func (d *Directories) RemoveExperiment(_ context.Context) error {
	recorded, settled, _, err := d.resolve()
	if err != nil {
		return err
	}
	if err := d.experimentLink().Rest(); err != nil {
		return err
	}
	settled.Experiment = nil
	if recorded.Experiment == nil && recorded.StableDeploymentID == settled.StableDeploymentID {
		// Nothing to change: a fresh install has no state file, and may have no state directory
		// yet.
		return nil
	}
	return d.writeState(settled)
}

// readState reads the state file, or returns the zero state when it, or its directory, is absent.
// Both must belong to the account this process runs as, root for the installer, and be writable by
// nobody else, and the file is opened
// without following a link: anything else is refused rather than read.
func (d *Directories) readState() (configState, error) {
	path := d.statePath()
	if err := checkStateDir(filepath.Dir(path)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configState{}, nil
		}
		return configState{}, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return configState{}, nil
	}
	if err != nil {
		return configState{}, fmt.Errorf("could not open the configuration state: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return configState{}, fmt.Errorf("could not stat the configuration state: %w", err)
	}
	if !info.Mode().IsRegular() {
		return configState{}, fmt.Errorf("refusing the configuration state %s: not a regular file", path)
	}
	if err := checkStateOwner(path, info); err != nil {
		return configState{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxStateSize+1))
	if err != nil {
		return configState{}, fmt.Errorf("could not read the configuration state: %w", err)
	}
	if len(raw) > maxStateSize {
		return configState{}, fmt.Errorf("refusing the configuration state %s: larger than %d bytes", path, maxStateSize)
	}
	var state configState
	if err := json.Unmarshal(raw, &state); err != nil {
		return configState{}, fmt.Errorf("could not parse the configuration state: %w", err)
	}
	return state, nil
}

// writeState replaces the state file through a temporary file renamed into place, so neither a
// reader nor a power loss ever sees a partial one.
func (d *Directories) writeState(state configState) error {
	path := d.statePath()
	if err := checkStateDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("refusing to write the configuration state: %w", err)
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("could not create the configuration state: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write the configuration state: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("could not set the configuration state's mode: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("could not flush the configuration state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close the configuration state: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}

// checkStateDir verifies that dir is a real directory owned by the account this process runs as and
// writable by nobody else, so nobody else can plant, swap or remove the state file in it.
func checkStateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("the configuration state directory %s is not a directory", dir)
	}
	return checkStateOwner(dir, info)
}

// checkStateOwner verifies that info belongs to the account this process runs as, root for every
// installer process, and is writable by nobody else.
func checkStateOwner(path string, info os.FileInfo) error {
	owner := os.Geteuid()
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != owner {
		return fmt.Errorf("refusing %s: not owned by uid %d", path, owner)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("refusing %s: writable by its group or by others (mode %s)", path, info.Mode().Perm())
	}
	return nil
}

// discardStaleScratch removes any leftover scratch directories from a WriteExperiment that
// crashed before cleaning up after itself. It is best effort: a failure here shouldn't stop a new
// experiment from being written.
func discardStaleScratch(parent string) {
	stale, err := filepath.Glob(filepath.Join(parent, incomingPrefix+"*"))
	if err != nil {
		log.Warnf("could not look for stale scratch configuration directories: %v", err)
		return
	}
	for _, dir := range stale {
		if err := os.RemoveAll(dir); err != nil {
			log.Warnf("could not discard the stale scratch configuration directory %s: %v", dir, err)
		}
	}
}

// setFileOwnershipAndPermissions sets the ownership and permissions for a file, through a handle
// that refuses to resolve to anything but the file itself. If the account doesn't exist (e.g. in
// tests) or the process doesn't have permission to change ownership, the function logs a warning
// and continues without failing.
//
// The owners and modes in the configFileSpec describe the Linux layout. On macOS every
// configuration file has the same ownership and mode, the ones the installer's .dmg set-up gives
// the whole tree, so a file written by Fleet is indistinguishable from one installed by the .dmg.
func setFileOwnershipAndPermissions(ctx context.Context, root *os.Root, path string, _ *configFileSpec) error {
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", path, err)
	}
	defer f.Close()
	if err := f.Chmod(agentConfigFileMode); err != nil {
		return fmt.Errorf("error setting file permissions for %s: %w", path, err)
	}
	uid, err := user.GetUserID(ctx, agentConfigUser)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
		return nil
	}
	gid, err := user.GetGroupID(ctx, agentConfigGroup)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
		return nil
	}
	if err := f.Chown(uid, gid); err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
	}
	return nil
}
