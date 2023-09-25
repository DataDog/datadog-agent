// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/fsnotify/fsnotify"
	"github.com/skydive-project/go-debouncer"
	"golang.org/x/exp/slices"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	workloadSelectorDebounceDelay = 5 * time.Second
	newFileDebounceDelay          = 2 * time.Second
)

var profileExtension = "." + config.Profile.String()

// make sure the DirectoryProvider implements Provider
var _ Provider = (*DirectoryProvider)(nil)

type profileFSEntry struct {
	path    string
	version string
}

// DirectoryProvider is a ProfileProvider that fetches Security Profiles from the filesystem
type DirectoryProvider struct {
	sync.Mutex
	directory      string
	watcherEnabled bool

	// attributes used by the inotify watcher
	cancelFnc         func()
	watcher           *fsnotify.Watcher
	newFilesDebouncer *debouncer.Debouncer
	newFiles          map[string]bool
	newFilesLock      sync.Mutex

	// we use a debouncer to forward new profiles to the profile manager in order to prevent a deadlock
	workloadSelectorDebouncer *debouncer.Debouncer
	onNewProfileCallback      func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)

	// selectors is used to select the profiles we currently care about
	selectors []cgroupModel.WorkloadSelector
	// profileMapping is an in-memory mapping of the profiles currently on the file system
	profileMapping map[cgroupModel.WorkloadSelector]profileFSEntry
}

// NewDirectoryProvider returns a new instance of DirectoryProvider
func NewDirectoryProvider(directory string, watch bool) (*DirectoryProvider, error) {
	// check if the provided directory exists
	if _, err := os.Stat(directory); err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(directory, 0750); err != nil {
				return nil, fmt.Errorf("can't create security profiles directory `%s`: %w", directory, err)
			}
		} else {
			return nil, fmt.Errorf("can't load security profiles from `%s`: %w", directory, err)
		}
	}

	dp := &DirectoryProvider{
		directory:      directory,
		watcherEnabled: watch,
		profileMapping: make(map[cgroupModel.WorkloadSelector]profileFSEntry),
		newFiles:       make(map[string]bool),
	}
	dp.workloadSelectorDebouncer = debouncer.New(workloadSelectorDebounceDelay, dp.onNewProfileDebouncerCallback)
	dp.newFilesDebouncer = debouncer.New(newFileDebounceDelay, dp.onHandleFilesFromWatcher)

	return dp, nil
}

// Start runs the directory provider
func (dp *DirectoryProvider) Start(ctx context.Context) error {
	dp.workloadSelectorDebouncer.Start()
	dp.newFilesDebouncer.Start()

	// add watches
	if dp.watcherEnabled {
		var err error
		if dp.watcher, err = fsnotify.NewWatcher(); err != nil {
			return fmt.Errorf("couldn't setup inotify watcher: %w", err)
		}

		if err = dp.watcher.Add(dp.directory); err != nil {
			_ = dp.watcher.Close()
			return err
		}

		var childContext context.Context
		childContext, dp.cancelFnc = context.WithCancel(ctx)
		go dp.watch(childContext)
	}

	// start by loading the profiles in the configured directory
	if err := dp.loadProfiles(); err != nil {
		return fmt.Errorf("couldn't scan the security profiles directory: %w", err)
	}
	return nil
}

// Stop closes the directory provider
func (dp *DirectoryProvider) Stop() error {
	dp.workloadSelectorDebouncer.Stop()
	dp.newFilesDebouncer.Stop()

	if dp.cancelFnc != nil {
		dp.cancelFnc()
	}

	if dp.watcher != nil {
		if err := dp.watcher.Close(); err != nil {
			seclog.Errorf("couldn't close profile watcher: %v", err)
		}
	}
	return nil
}

// UpdateWorkloadSelectors updates the selectors used to query profiles
func (dp *DirectoryProvider) UpdateWorkloadSelectors(selectors []cgroupModel.WorkloadSelector) {
	dp.Lock()
	defer dp.Unlock()
	dp.selectors = selectors

	if dp.onNewProfileCallback == nil {
		return
	}

	dp.workloadSelectorDebouncer.Call()
}

func (dp *DirectoryProvider) onNewProfileDebouncerCallback() {
	for _, selector := range dp.selectors {
		for profileSelector, profilePath := range dp.profileMapping {
			if selector.Match(profileSelector) {
				// read and parse profile
				profile, err := LoadProfileFromFile(profilePath.path)
				if err != nil {
					seclog.Warnf("couldn't load profile %s: %v", profilePath, err)
					continue
				}

				// propagate the new profile
				dp.onNewProfileCallback(profileSelector, profile)
			}
		}
	}
}

// SetOnNewProfileCallback sets the onNewProfileCallback function
func (dp *DirectoryProvider) SetOnNewProfileCallback(onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)) {
	dp.onNewProfileCallback = onNewProfileCallback
}

func (dp *DirectoryProvider) listProfiles() ([]string, error) {
	files, err := os.ReadDir(dp.directory)
	if err != nil {
		return nil, err
	}

	var output []string
	for _, profilePath := range files {
		name := profilePath.Name()

		if filepath.Ext(name) != profileExtension {
			continue
		}

		output = append(output, filepath.Join(dp.directory, name))
	}

	sort.Slice(output, func(i, j int) bool {
		return output[i] < output[j]
	})
	return output, nil
}

func (dp *DirectoryProvider) loadProfile(profilePath string) error {
	profile, err := LoadProfileFromFile(profilePath)
	if err != nil {
		return fmt.Errorf("couldn't load profile %s: %w", profilePath, err)
	}
	workloadSelector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", profile.Tags), utils.GetTagValue("image_tag", profile.Tags))
	if err != nil {
		return err
	}

	// lock selectors and profiles mapping
	dp.Lock()
	defer dp.Unlock()

	// update profile mapping
	if existingProfile, ok := dp.profileMapping[workloadSelector]; ok {
		if existingProfile.version > profile.Version {
			seclog.Warnf("ignoring %s (version: %v status: %s): a more recent version of this profile already exists (existing version is %v)", profilePath, profile.Version, model.Status(profile.Status), existingProfile.version)
			return nil
		}
	}
	dp.profileMapping[workloadSelector] = profileFSEntry{
		path:    profilePath,
		version: profile.Version,
	}

	seclog.Debugf("security profile %s (version: %s status: %s) loaded from file system", workloadSelector, profile.Version, model.Status(profile.Status))

	if dp.onNewProfileCallback == nil {
		return nil
	}

	// check if this profile matches a workload selector
	for _, selector := range dp.selectors {
		if workloadSelector.Match(selector) {
			dp.onNewProfileCallback(workloadSelector, profile)
		}
	}
	return nil
}

func (dp *DirectoryProvider) loadProfiles() error {
	files, err := dp.listProfiles()
	if err != nil {
		return err
	}

	for _, profilePath := range files {
		if err = dp.loadProfile(profilePath); err != nil {
			seclog.Errorf("couldn't load profile: %v", err)
		}
	}
	return nil
}

func (dp *DirectoryProvider) findProfile(path string) (cgroupModel.WorkloadSelector, bool) {
	dp.Lock()
	defer dp.Unlock()

	for selector, profile := range dp.profileMapping {
		if path == profile.path {
			return selector, true
		}
	}
	return cgroupModel.WorkloadSelector{}, false
}

func (dp *DirectoryProvider) getProfiles() map[cgroupModel.WorkloadSelector]profileFSEntry {
	dp.Lock()
	defer dp.Unlock()
	return dp.profileMapping
}

// OnLocalStorageCleanup removes the provided files from the entries of the directory provider
func (dp *DirectoryProvider) OnLocalStorageCleanup(files []string) {
	dp.Lock()
	defer dp.Unlock()

	fileMask := make(map[string]bool)
	for _, file := range files {
		if path.Ext(file) == profileExtension {
			fileMask[file] = true
		}
	}

	// iterate through the list of profiles and remove the entries that are about to be deleted from the file system
	for selector, fsEntry := range dp.profileMapping {
		if _, found := fileMask[fsEntry.path]; found {
			delete(dp.profileMapping, selector)
			delete(fileMask, fsEntry.path)
			if len(fileMask) == 0 {
				break
			}
		}
	}
}

func (dp *DirectoryProvider) deleteProfile(selector cgroupModel.WorkloadSelector) {
	dp.Lock()
	defer dp.Unlock()
	delete(dp.profileMapping, selector)
}

func (dp *DirectoryProvider) onHandleFilesFromWatcher() {
	dp.newFilesLock.Lock()
	defer dp.newFilesLock.Unlock()

	for file := range dp.newFiles {
		if err := dp.loadProfile(file); err != nil {
			if errors.Is(err, cgroupModel.ErrNoImageProvided) {
				seclog.Debugf("couldn't load new profile %s: %v", file, err)
			} else {
				seclog.Errorf("couldn't load new profile %s: %v", file, err)
			}

			continue
		}
	}

	dp.newFiles = make(map[string]bool)
}

func (dp *DirectoryProvider) watch(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-dp.watcher.Events:
				if !ok {
					return
				}

				if event.Op&(fsnotify.Create|fsnotify.Remove) > 0 {
					files, err := dp.listProfiles()
					if err != nil {
						seclog.Errorf("couldn't list profiles: %v", err)
						continue
					}

					if event.Op&fsnotify.Create > 0 {
						// look for the new profile
						for _, file := range files {
							if _, ok = dp.findProfile(file); ok {
								continue
							}

							// add file in the list of new files
							dp.newFilesLock.Lock()
							dp.newFiles[file] = true
							dp.newFilesLock.Unlock()
							dp.newFilesDebouncer.Call()
						}
					} else if event.Op&fsnotify.Remove > 0 {
						// look for the deleted profile
						for selector, profile := range dp.getProfiles() {
							if slices.Contains(files, profile.path) {
								continue
							}

							// delete profile
							dp.deleteProfile(selector)

							seclog.Debugf("security profile %s (version %s) removed from profile mapping", selector, profile.version)
						}
					}

				} else if event.Op&fsnotify.Write > 0 && filepath.Ext(event.Name) == profileExtension {
					// add file in the list of new files
					dp.newFilesLock.Lock()
					dp.newFiles[event.Name] = true
					dp.newFilesLock.Unlock()
					dp.newFilesDebouncer.Call()
				}
			case _, ok := <-dp.watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// SendStats sends the metrics of the directory provider
func (dp *DirectoryProvider) SendStats(client statsd.ClientInterface) error {
	dp.Lock()
	defer dp.Unlock()

	if value := len(dp.profileMapping); value > 0 {
		if err := client.Gauge(metrics.MetricSecurityProfileDirectoryProviderCount, float64(value), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricSecurityProfileDirectoryProviderCount, err)
		}
	}

	return nil
}
