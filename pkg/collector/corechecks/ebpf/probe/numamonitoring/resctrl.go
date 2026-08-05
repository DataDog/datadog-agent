// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package numamonitoring

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/numamonitoring/model"
)

const agentGroupPrefix = "dd-numa-"

const (
	featureLLCOccupancy = "llc_occupancy"
	featureMBMTotal     = "mbm_total_bytes"
	featureMBMLocal     = "mbm_local_bytes"
)

type counterSample struct {
	value uint64
	at    time.Time
}

// resctrlManager only manages root-level monitor groups whose names begin with
// agentGroupPrefix. It deliberately has no mount, schemata, CAT, MBA, or event
// configuration code.
type resctrlManager struct {
	root       string
	features   []string
	maxGroups  int
	groups     map[uint64][]int
	previous   map[string]counterSample
	conflicts  uint64
	readErrors uint64
}

func newResctrlManager(root string, maxGroups int) *resctrlManager {
	manager := &resctrlManager{
		root:      root,
		maxGroups: maxGroups,
		groups:    make(map[uint64][]int),
		previous:  make(map[string]counterSample),
	}
	manager.features = readMonitorFeatures(root)
	manager.reclaimOwnedGroups()
	return manager
}

func readMonitorFeatures(root string) []string {
	contents, err := os.ReadFile(filepath.Join(root, "info", "L3_MON", "mon_features"))
	if err != nil {
		return nil
	}
	var features []string
	for _, feature := range strings.Fields(string(contents)) {
		switch feature {
		case featureLLCOccupancy, featureMBMTotal, featureMBMLocal:
			features = append(features, feature)
		}
	}
	slices.Sort(features)
	return slices.Compact(features)
}

func (manager *resctrlManager) supported() bool {
	if len(manager.features) == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(manager.root, "mon_groups"))
	return err == nil && info.IsDir()
}

func (manager *resctrlManager) groupPath(cgroupID uint64) string {
	return filepath.Join(manager.root, "mon_groups", fmt.Sprintf("%s%d", agentGroupPrefix, cgroupID))
}

func (manager *resctrlManager) reclaimOwnedGroups() {
	groups, err := filepath.Glob(filepath.Join(manager.root, "mon_groups", agentGroupPrefix+"*"))
	if err != nil {
		return
	}
	for _, group := range groups {
		if err := manager.removeOwnedGroup(group, nil); err != nil {
			manager.readErrors++
		}
	}
}

func (manager *resctrlManager) removeOwnedGroup(group string, tasks []int) error {
	if !strings.HasPrefix(filepath.Base(group), agentGroupPrefix) {
		return fmt.Errorf("refusing to remove foreign resctrl group %q", group)
	}
	for _, tid := range tasks {
		if err := writeTask(filepath.Join(manager.root, "tasks"), tid); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	err := os.Remove(group)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// Regular-directory fake filesystems do not implement resctrl's virtual
	// rmdir behavior. Remove only known files below an Agent-owned fixture.
	if removeErr := removeOwnedFixture(group); removeErr != nil {
		return errors.Join(err, removeErr)
	}
	return nil
}

func (manager *resctrlManager) clearPrevious(cgroupID uint64) {
	prefix := strconv.FormatUint(cgroupID, 10) + "/"
	for key := range manager.previous {
		if strings.HasPrefix(key, prefix) {
			delete(manager.previous, key)
		}
	}
}

func removeOwnedFixture(group string) error {
	if !strings.HasPrefix(filepath.Base(group), agentGroupPrefix) {
		return fmt.Errorf("refusing to remove foreign resctrl fixture %q", group)
	}
	return os.RemoveAll(group)
}

func writeTask(tasksPath string, tid int) error {
	file, err := os.OpenFile(tasksPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(strconv.Itoa(tid))
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func writeGroupTask(group string, tid int) error {
	if !strings.HasPrefix(filepath.Base(group), agentGroupPrefix) {
		return fmt.Errorf("refusing to write foreign resctrl group %q", group)
	}
	tasksPath := filepath.Join(group, "tasks")
	err := writeTask(tasksPath, tid)
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// A real resctrl group creates tasks automatically. This fallback exists
	// only for regular-directory test fixtures and is constrained to an
	// Agent-owned group.
	file, createErr := os.OpenFile(tasksPath, os.O_WRONLY|os.O_CREATE, 0o600)
	if createErr != nil {
		return createErr
	}
	_, writeErr := file.WriteString(strconv.Itoa(tid))
	return errors.Join(writeErr, file.Close())
}

func (manager *resctrlManager) foreignTasks() map[int]struct{} {
	foreign := make(map[int]struct{})
	rootTasks := filepath.Clean(filepath.Join(manager.root, "tasks"))
	err := filepath.WalkDir(manager.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "tasks" {
			return nil
		}
		cleanPath := filepath.Clean(path)
		if cleanPath == rootTasks || strings.HasPrefix(filepath.Base(filepath.Dir(cleanPath)), agentGroupPrefix) {
			return nil
		}
		contents, readErr := os.ReadFile(cleanPath)
		if readErr != nil {
			manager.readErrors++
			return nil
		}
		for _, value := range strings.Fields(string(contents)) {
			tid, parseErr := strconv.Atoi(value)
			if parseErr == nil {
				foreign[tid] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		manager.readErrors++
	}
	return foreign
}

func (manager *resctrlManager) rotate(selected map[uint64][]int) {
	if !manager.supported() {
		return
	}
	manager.conflicts = 0
	foreign := manager.foreignTasks()

	for cgroupID, tasks := range manager.groups {
		if _, keep := selected[cgroupID]; keep {
			continue
		}
		if err := manager.removeOwnedGroup(manager.groupPath(cgroupID), tasks); err != nil {
			manager.readErrors++
		}
		delete(manager.groups, cgroupID)
		manager.clearPrevious(cgroupID)
	}

	// Move removed tasks to the root before assigning any task to its new
	// group. This ordering is important when a task moves between two groups.
	for cgroupID, oldTasks := range manager.groups {
		newTasks, keep := selected[cgroupID]
		if !keep {
			continue
		}
		newSet := make(map[int]struct{}, len(newTasks))
		for _, tid := range newTasks {
			newSet[tid] = struct{}{}
		}
		for _, tid := range oldTasks {
			if _, found := newSet[tid]; !found {
				if err := writeTask(filepath.Join(manager.root, "tasks"), tid); err != nil {
					manager.readErrors++
				}
			}
		}
	}

	ids := make([]uint64, 0, len(selected))
	for cgroupID := range selected {
		ids = append(ids, cgroupID)
	}
	slices.Sort(ids)
	for _, cgroupID := range ids {
		tasks := selected[cgroupID]
		conflict := false
		for _, tid := range tasks {
			if _, found := foreign[tid]; found {
				conflict = true
				manager.conflicts++
			}
		}
		if conflict {
			if oldTasks, active := manager.groups[cgroupID]; active {
				if err := manager.removeOwnedGroup(manager.groupPath(cgroupID), oldTasks); err != nil {
					manager.readErrors++
				}
				delete(manager.groups, cgroupID)
				manager.clearPrevious(cgroupID)
			}
			continue
		}

		group := manager.groupPath(cgroupID)
		_, active := manager.groups[cgroupID]
		if !active {
			if len(manager.groups) >= manager.maxGroups {
				continue
			}
			if err := os.Mkdir(group, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				manager.readErrors++
				continue
			}
		}
		ok := true
		for _, tid := range tasks {
			if err := writeGroupTask(group, tid); err != nil {
				manager.readErrors++
				ok = false
				break
			}
		}
		if ok {
			manager.groups[cgroupID] = slices.Clone(tasks)
		}
	}
}

func (manager *resctrlManager) read(cgroupID uint64, now time.Time) []model.DomainStats {
	if _, active := manager.groups[cgroupID]; !active {
		return nil
	}
	domains, err := filepath.Glob(filepath.Join(manager.groupPath(cgroupID), "mon_data", "mon_L3_*"))
	if err != nil {
		manager.readErrors++
		return nil
	}
	result := make([]model.DomainStats, 0, len(domains))
	for _, domainPath := range domains {
		domain := strings.TrimPrefix(filepath.Base(domainPath), "mon_L3_")
		stats := model.DomainStats{Domain: domain}
		var total, local *float64
		for _, feature := range manager.features {
			value, available, readErr := readCounter(filepath.Join(domainPath, feature))
			if readErr != nil {
				manager.readErrors++
				continue
			}
			if !available {
				continue
			}
			if feature == featureLLCOccupancy {
				occupancy := float64(value)
				stats.LLCOccupancy = &occupancy
				continue
			}

			key := fmt.Sprintf("%d/%s/%s", cgroupID, domain, feature)
			previous, found := manager.previous[key]
			manager.previous[key] = counterSample{value: value, at: now}
			if !found {
				continue
			}
			rate, valid := counterRate(previous.value, value, now.Sub(previous.at))
			if !valid {
				continue
			}
			switch feature {
			case featureMBMTotal:
				stats.TotalBandwidth = &rate
				total = &rate
			case featureMBMLocal:
				stats.LocalBandwidth = &rate
				local = &rate
			}
		}
		if total != nil && local != nil {
			remote, _, valid := remoteRatio(*total, *local)
			if valid {
				stats.RemoteBandwidth = &remote
			}
		}
		result = append(result, stats)
	}
	return result
}

func readCounter(path string) (uint64, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	value := strings.TrimSpace(string(contents))
	if strings.EqualFold(value, "unavailable") || value == "" {
		return 0, false, nil
	}
	counter, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse resctrl counter %s: %w", path, err)
	}
	return counter, true, nil
}

func (manager *resctrlManager) close() {
	for cgroupID, tasks := range manager.groups {
		if err := manager.removeOwnedGroup(manager.groupPath(cgroupID), tasks); err != nil {
			manager.readErrors++
		}
	}
	clear(manager.groups)
	clear(manager.previous)
}
