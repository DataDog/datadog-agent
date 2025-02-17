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
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/fsnotify/fsnotify"
	"github.com/skydive-project/go-debouncer"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

var (
	workloadSelectorDebounceDelay = 5 * time.Second
	newFileDebounceDelay          = 2 * time.Second
)

var profileExtension = "." + config.Profile.String()

// make sure the DirectoryProvider implements Provider
var _ Provider = (*DirectoryProvider)(nil)

type profileFSEntry struct {
	path     string
	selector cgroupModel.WorkloadSelector
}

type existingProfileInfo struct {
	path     string
	selector string
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
	// we don't want to keep the lock for too long, especially not while calling the callback
	dp.Lock()
	selectors := make([]cgroupModel.WorkloadSelector, len(dp.selectors))
	copy(selectors, dp.selectors)
	profileMapping := maps.Clone(dp.profileMapping)
	propagateCb := dp.onNewProfileCallback
	dp.Unlock()

	if propagateCb == nil {
		return
	}

	for _, selector := range selectors {
		for profileSelector, profilePath := range profileMapping {
			if selector.Match(profileSelector) {
				// read and parse profile
				profile, err := LoadProtoFromFile(profilePath.path)
				if err != nil {
					seclog.Warnf("couldn't load profile %s: %v", profilePath.path, err)
					continue
				}

				// propagate the new profile
				propagateCb(profileSelector, profile)
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

func (dp *DirectoryProvider) loadProfile(profilePath string) (*existingProfileInfo, error) {
	profile, err := LoadProtoFromFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("couldn't load profile %s: %w", profilePath, err)
	}

	if len(profile.ProfileContexts) == 0 {
		return nil, fmt.Errorf("couldn't load profile %s: it did not contains any version", profilePath)
	}

	imageName, imageTag := profile.Selector.GetImageName(), profile.Selector.GetImageTag()
	if imageTag == "" || imageName == "" {
		return nil, fmt.Errorf("couldn't load profile %s: it did not contains any valid image_name (%s) or image_tag (%s)", profilePath, imageName, imageTag)
	}

	workloadSelector, err := cgroupModel.NewWorkloadSelector(imageName, imageTag)
	if err != nil {
		return nil, err
	}
	profileManagerSelector := workloadSelector
	profileManagerSelector.Tag = "*"

	// lock selectors and profiles mapping
	dp.Lock()

	// prioritize a persited profile over activity dumps
	if _, ok := dp.profileMapping[profileManagerSelector]; ok {
		dp.Unlock()
		return &existingProfileInfo{path: profilePath, selector: profileManagerSelector.String()}, nil
	}

	// update profile mapping
	dp.profileMapping[profileManagerSelector] = profileFSEntry{
		path:     profilePath,
		selector: workloadSelector,
	}

	selectors := make([]cgroupModel.WorkloadSelector, len(dp.selectors))
	copy(selectors, dp.selectors)
	propagateCb := dp.onNewProfileCallback

	// Unlock before calling the callback to avoid deadlocks
	dp.Unlock()

	seclog.Debugf("security profile %s loaded from file system", workloadSelector)

	if propagateCb == nil {
		return nil, nil
	}

	// check if this profile matches a workload selector
	for _, selector := range selectors {
		if workloadSelector.Match(selector) {
			propagateCb(workloadSelector, profile)
		}
	}
	return nil, nil
}

func (dp *DirectoryProvider) loadProfiles() error {
	files, err := dp.listProfiles()
	if err != nil {
		return err
	}

	for _, profilePath := range files {
		existingProfile, err := dp.loadProfile(profilePath)
		if err != nil {
			seclog.Errorf("couldn't load profile: %v", err)
		} else if existingProfile != nil {
			seclog.Debugf("ignoring %s: a persisted profile already exists for workload %s", existingProfile.path, existingProfile.selector)
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

	var filesToCleanup []string
	for file := range dp.newFiles {
		existingProfile, err := dp.loadProfile(file)
		if err != nil {
			if errors.Is(err, cgroupModel.ErrNoImageProvided) {
				seclog.Debugf("couldn't load new profile %s: %v", file, err)
			} else {
				seclog.Warnf("couldn't load new profile %s: %v", file, err)
			}
			filesToCleanup = append(filesToCleanup, file)
		} else if existingProfile != nil {
			seclog.Debugf("ignoring %s: a persisted profile already exists for workload %s", existingProfile.path, existingProfile.selector)
			filesToCleanup = append(filesToCleanup, file)
		}
	}

	if len(filesToCleanup) != 0 {
		dp.OnLocalStorageCleanup(filesToCleanup)
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

				if event.Has(fsnotify.Create | fsnotify.Remove) {
					files, err := dp.listProfiles()
					if err != nil {
						seclog.Errorf("couldn't list profiles: %v", err)
						continue
					}

					if event.Has(fsnotify.Create) {
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
					} else if event.Has(fsnotify.Remove) {
						// look for the deleted profile
						for selector, profile := range dp.getProfiles() {
							if slices.Contains(files, profile.path) {
								continue
							}

							// delete profile
							dp.deleteProfile(selector)
							dp.newFilesLock.Lock()
							delete(dp.newFiles, profile.path)
							dp.newFilesLock.Unlock()

							seclog.Debugf("security profile %s removed from profile mapping", selector)
						}
					}

				} else if event.Has(fsnotify.Write) && filepath.Ext(event.Name) == profileExtension {
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
