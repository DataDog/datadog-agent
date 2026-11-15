// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/godbus/dbus/v5"
)

const (
	systemdBusDestination = "org.freedesktop.systemd1"
	systemdManagerPath    = dbus.ObjectPath("/org/freedesktop/systemd1")
	systemdManagerIface   = "org.freedesktop.systemd1.Manager"
	systemdUnitIface      = "org.freedesktop.systemd1.Unit"
	systemdServiceIface   = "org.freedesktop.systemd1.Service"
	dbusPeerGetMachineID  = "org.freedesktop.DBus.Peer.GetMachineId"
	dbusPropertiesGet     = "org.freedesktop.DBus.Properties.Get"

	systemdUnitPathPrefix = "/org/freedesktop/systemd1/unit/"
	systemdJobPathPrefix  = "/org/freedesktop/systemd1/job/"
	maxManagerObjectPath  = 4096
	maxUnitFileChanges    = 256
	maxManagerSignalQueue = 64
	maxManagerSignalsRead = 256
)

type managerBus interface {
	call(ctx context.Context, destination string, path dbus.ObjectPath, method string, arguments ...any) ([]any, error)
	addJobRemovedMatch(ctx context.Context) error
	registerSignals(channel chan<- *dbus.Signal) <-chan struct{}
	removeSignals(channel chan<- *dbus.Signal)
}

func verifySystemdManagerMachineID(ctx context.Context, bus managerBus, expectedMachineID string) error {
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, dbusPeerGetMachineID)
	if err != nil {
		return managerMethodError("Peer.GetMachineId", "", err)
	}
	var actualMachineID string
	if err := storeManagerReply(body, &actualMachineID); err != nil {
		return fmt.Errorf("systemd manager peer returned an invalid machine ID reply: %w", err)
	}
	if len(actualMachineID) != 32 || !validID128(actualMachineID) {
		return errors.New("systemd manager peer returned an invalid machine ID")
	}
	if !strings.EqualFold(actualMachineID, expectedMachineID) {
		return errors.New("systemd manager peer machine ID does not match the configured target")
	}
	return nil
}

type unitJobProperty struct {
	ID   uint32
	Path dbus.ObjectPath
}

type unitFileChange struct {
	Type        string
	Destination string
	Source      string
}

func listSystemServicesWithBus(ctx context.Context, bus managerBus, request builtins.SystemServiceListRequest) ([]builtins.SystemServiceState, error) {
	if err := validateManagerUnits(request.Services, true); err != nil {
		return nil, err
	}
	states := make([]builtins.SystemServiceState, 0, len(request.Services))
	for _, unit := range request.Services {
		state, found, err := inspectSystemUnit(ctx, bus, unit, request.IncludeInactive, true, true, false)
		if err != nil {
			return nil, err
		}
		if found && (request.IncludeInactive || state.ActiveState != "inactive" || state.JobID != 0) {
			states = append(states, state)
		}
	}
	if len(states) > len(request.Services) {
		return nil, errors.New("systemd manager returned more units than requested")
	}
	return states, nil
}

func inspectSystemServicesWithBus(ctx context.Context, bus managerBus, units []string) ([]builtins.SystemServiceState, error) {
	if err := validateManagerUnits(units, false); err != nil {
		return nil, err
	}
	states := make([]builtins.SystemServiceState, 0, len(units))
	for _, unit := range units {
		state, found, err := inspectSystemUnit(ctx, bus, unit, true, true, false, true)
		if err != nil {
			return nil, err
		}
		if !found {
			state = builtins.SystemServiceState{
				Name:        unit,
				LoadState:   "not-found",
				ActiveState: "inactive",
				SubState:    "dead",
			}
		}
		states = append(states, state)
	}
	if len(states) != len(units) {
		return nil, errors.New("systemd manager returned an unexpected unit count")
	}
	return states, nil
}

func inspectSystemUnit(ctx context.Context, bus managerBus, selector string, load, allowMissing, includeJob, includeDetails bool) (builtins.SystemServiceState, bool, error) {
	method := "GetUnit"
	if load {
		method = "LoadUnit"
	}
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+"."+method, selector)
	if err != nil {
		if allowMissing && isNoSuchUnitError(err) {
			return builtins.SystemServiceState{}, false, nil
		}
		return builtins.SystemServiceState{}, false, managerMethodError(method, selector, err)
	}
	var path dbus.ObjectPath
	if err := storeManagerReply(body, &path); err != nil {
		return builtins.SystemServiceState{}, false, fmt.Errorf("systemd manager %s returned an invalid reply for %q: %w", method, selector, err)
	}
	if err := validateManagerObjectPath("unit", path, systemdUnitPathPrefix); err != nil {
		return builtins.SystemServiceState{}, false, err
	}

	state := builtins.SystemServiceState{Name: selector}
	if state.CanonicalName, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "Id", false); err != nil {
		return builtins.SystemServiceState{}, false, err
	}
	if err := validateManagerCanonicalUnit(state.CanonicalName); err != nil {
		return builtins.SystemServiceState{}, false, fmt.Errorf("systemd manager returned an invalid canonical unit name: %w", err)
	}
	if state.Description, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "Description", true); err != nil {
		return builtins.SystemServiceState{}, false, err
	}
	if state.LoadState, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "LoadState", false); err != nil {
		return builtins.SystemServiceState{}, false, err
	}
	if state.ActiveState, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "ActiveState", false); err != nil {
		return builtins.SystemServiceState{}, false, err
	}
	if state.SubState, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "SubState", false); err != nil {
		return builtins.SystemServiceState{}, false, err
	}
	if state.LoadState == "not-found" {
		return builtins.SystemServiceState{}, false, nil
	}
	if includeJob {
		job, err := managerJobProperty(ctx, bus, path)
		if err != nil {
			return builtins.SystemServiceState{}, false, err
		}
		state.JobID = job.ID
		if job.ID == 0 {
			if job.Path != "/" {
				return builtins.SystemServiceState{}, false, errors.New("systemd manager returned a zero job with an unexpected object path")
			}
		} else if err := validateManagerJobPath(job.Path, job.ID); err != nil {
			return builtins.SystemServiceState{}, false, err
		}
	}

	if includeDetails {
		if state.UnitFileState, err = managerStringProperty(ctx, bus, path, systemdUnitIface, "UnitFileState", true); err != nil {
			return builtins.SystemServiceState{}, false, err
		}
		if resultInterface, ok := managerResultInterface(state.CanonicalName); ok {
			if state.Result, err = managerStringProperty(ctx, bus, path, resultInterface, "Result", true); err != nil {
				return builtins.SystemServiceState{}, false, err
			}
		}
	}
	if includeDetails && strings.HasSuffix(state.CanonicalName, ".service") {
		if state.MainPID, err = managerUint32Property(ctx, bus, path, systemdServiceIface, "MainPID"); err != nil {
			return builtins.SystemServiceState{}, false, err
		}
	}
	return state, true, nil
}

func managerResultInterface(unit string) (string, bool) {
	switch {
	case strings.HasSuffix(unit, ".service"):
		return systemdServiceIface, true
	case strings.HasSuffix(unit, ".socket"):
		return "org.freedesktop.systemd1.Socket", true
	case strings.HasSuffix(unit, ".mount"):
		return "org.freedesktop.systemd1.Mount", true
	case strings.HasSuffix(unit, ".automount"):
		return "org.freedesktop.systemd1.Automount", true
	case strings.HasSuffix(unit, ".timer"):
		return "org.freedesktop.systemd1.Timer", true
	case strings.HasSuffix(unit, ".swap"):
		return "org.freedesktop.systemd1.Swap", true
	case strings.HasSuffix(unit, ".path"):
		return "org.freedesktop.systemd1.Path", true
	case strings.HasSuffix(unit, ".scope"):
		return "org.freedesktop.systemd1.Scope", true
	default:
		return "", false
	}
}

func runSystemServiceJobsWithBus(ctx context.Context, bus managerBus, action builtins.SystemServiceJobAction, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	method, ok := managerJobMethod(action)
	if !ok {
		return fmt.Errorf("unsupported systemd manager job action %q", action)
	}

	signals := make(chan *dbus.Signal, maxManagerSignalQueue)
	overflow := bus.registerSignals(signals)
	defer bus.removeSignals(signals)
	if err := bus.addJobRemovedMatch(ctx); err != nil {
		return fmt.Errorf("subscribe to systemd manager job results: %w", err)
	}
	// Intentionally unpaired: there is no matching Unsubscribe call. systemd
	// drops the subscription when the client disconnects, and withManagerBus
	// always closes the connection before returning, so teardown is by
	// connection close. Adding Unsubscribe would widen a manager surface that
	// docs/RULES.md deliberately keeps fixed and minimal, for no benefit.
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+".Subscribe")
	if err != nil {
		return managerMethodError("Subscribe", "", err)
	}
	if err := storeManagerReply(body); err != nil {
		return fmt.Errorf("systemd manager Subscribe returned an invalid reply: %w", err)
	}

	for index, unit := range units {
		body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+"."+method, unit, "replace")
		if err != nil {
			methodErr := managerMethodError(method, unit, err)
			if managerDBusErrorName(err) == "" {
				methodErr = managerJobDeliveryUncertain(unit, methodErr)
			}
			return managerPartialProgress(units[:index], methodErr)
		}
		var jobPath dbus.ObjectPath
		if err := storeManagerReply(body, &jobPath); err != nil {
			return managerPartialProgress(units[:index], managerJobOutcomeUncertain(unit, fmt.Errorf("systemd manager %s returned an invalid reply: %w", method, err)))
		}
		if err := validateReturnedManagerJobPath(jobPath); err != nil {
			return managerPartialProgress(units[:index], managerJobOutcomeUncertain(unit, err))
		}
		if err := waitSystemdJob(ctx, signals, overflow, jobPath, unit); err != nil {
			return managerPartialProgress(units[:index], managerJobOutcomeUncertain(unit, err))
		}
	}
	return nil
}

func enableSystemServicesWithBus(ctx context.Context, bus managerBus, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+".EnableUnitFiles", units, false, false)
	if err != nil {
		return fmt.Errorf("systemd manager EnableUnitFiles failed; unit-file changes may have been partially applied and were not rolled back: %w", managerMethodError("EnableUnitFiles", "", err))
	}
	var carriesInstallInfo bool
	var changes []unitFileChange
	if err := storeManagerReply(body, &carriesInstallInfo, &changes); err != nil {
		return unitFilePartialProgress(fmt.Errorf("systemd manager EnableUnitFiles returned an invalid reply: %w", err))
	}
	if err := validateUnitFileChanges(changes); err != nil {
		return unitFilePartialProgress(err)
	}
	if err := reloadSystemdManager(ctx, bus); err != nil {
		return unitFilePartialProgress(err)
	}
	return nil
}

func disableSystemServicesWithBus(ctx context.Context, bus managerBus, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+".DisableUnitFiles", units, false)
	if err != nil {
		return fmt.Errorf("systemd manager DisableUnitFiles failed; unit-file changes may have been partially applied and were not rolled back: %w", managerMethodError("DisableUnitFiles", "", err))
	}
	var changes []unitFileChange
	if err := storeManagerReply(body, &changes); err != nil {
		return unitFilePartialProgress(fmt.Errorf("systemd manager DisableUnitFiles returned an invalid reply: %w", err))
	}
	if err := validateUnitFileChanges(changes); err != nil {
		return unitFilePartialProgress(err)
	}
	if err := reloadSystemdManager(ctx, bus); err != nil {
		return unitFilePartialProgress(err)
	}
	return nil
}

func reloadSystemdManager(ctx context.Context, bus managerBus) error {
	body, err := bus.call(ctx, systemdBusDestination, systemdManagerPath, systemdManagerIface+".Reload")
	if err != nil {
		return managerMethodError("Reload", "", err)
	}
	if err := storeManagerReply(body); err != nil {
		return fmt.Errorf("systemd manager Reload returned an invalid reply: %w", err)
	}
	return nil
}

func unitFilePartialProgress(err error) error {
	return fmt.Errorf("unit-file changes completed, but manager reload/validation failed; changes were not rolled back: %w", err)
}

func managerPartialProgress(completed []string, err error) error {
	if len(completed) == 0 {
		return err
	}
	return fmt.Errorf("systemd manager operation stopped after completing %d units (%s); completed operations were not rolled back: %w", len(completed), strings.Join(completed, ", "), err)
}

func managerJobOutcomeUncertain(unit string, err error) error {
	return fmt.Errorf("systemd manager accepted a job for %q, but its final state is unknown and was not rolled back: %w", unit, err)
}

func managerJobDeliveryUncertain(unit string, err error) error {
	return fmt.Errorf("systemd manager may have accepted a job for %q, but its outcome is unknown and was not rolled back: %w", unit, err)
}

func managerJobMethod(action builtins.SystemServiceJobAction) (string, bool) {
	switch action {
	case builtins.SystemServiceJobStart:
		return "StartUnit", true
	case builtins.SystemServiceJobStop:
		return "StopUnit", true
	case builtins.SystemServiceJobReload:
		return "ReloadUnit", true
	case builtins.SystemServiceJobRestart:
		return "RestartUnit", true
	default:
		return "", false
	}
}

func waitSystemdJob(ctx context.Context, signals <-chan *dbus.Signal, overflow <-chan struct{}, expectedPath dbus.ObjectPath, selector string) error {
	for examined := 0; examined < maxManagerSignalsRead; examined++ {
		select {
		case <-overflow:
			return fmt.Errorf("systemd manager signal queue overflowed while waiting for %q", selector)
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-overflow:
			return fmt.Errorf("systemd manager signal queue overflowed while waiting for %q", selector)
		case signal, ok := <-signals:
			if !ok {
				return fmt.Errorf("systemd manager connection closed while waiting for %q", selector)
			}
			if signal == nil {
				return errors.New("systemd manager returned a nil job signal")
			}
			if signal.Name != systemdManagerIface+".JobRemoved" || signal.Path != systemdManagerPath {
				continue
			}
			var jobID uint32
			var jobPath dbus.ObjectPath
			var unit, result string
			if err := dbus.Store(signal.Body, &jobID, &jobPath, &unit, &result); err != nil {
				return fmt.Errorf("systemd manager JobRemoved signal has an invalid body: %w", err)
			}
			if err := validateManagerJobPath(jobPath, jobID); err != nil {
				return err
			}
			if err := validateManagerCanonicalUnit(unit); err != nil {
				return fmt.Errorf("systemd manager JobRemoved signal has an invalid unit: %w", err)
			}
			if err := validateManagerString("job result", result, false); err != nil {
				return err
			}
			if jobPath != expectedPath {
				continue
			}
			select {
			case <-overflow:
				return fmt.Errorf("systemd manager signal queue overflowed while waiting for %q", selector)
			default:
			}
			if result != "done" && result != "skipped" {
				return fmt.Errorf("systemd manager job for %q finished with result %q", selector, result)
			}
			return nil
		}
	}
	return fmt.Errorf("systemd manager emitted too many unrelated job results while waiting for %q", selector)
}

func managerStringProperty(ctx context.Context, bus managerBus, path dbus.ObjectPath, iface, property string, allowEmpty bool) (string, error) {
	var value string
	if err := managerProperty(ctx, bus, path, iface, property, &value); err != nil {
		return "", err
	}
	if err := validateManagerString(property, value, allowEmpty); err != nil {
		return "", fmt.Errorf("systemd manager returned an invalid %s property: %w", property, err)
	}
	return value, nil
}

func managerUint32Property(ctx context.Context, bus managerBus, path dbus.ObjectPath, iface, property string) (uint32, error) {
	var value uint32
	if err := managerProperty(ctx, bus, path, iface, property, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func managerJobProperty(ctx context.Context, bus managerBus, path dbus.ObjectPath) (unitJobProperty, error) {
	var value unitJobProperty
	if err := managerProperty(ctx, bus, path, systemdUnitIface, "Job", &value); err != nil {
		return unitJobProperty{}, err
	}
	return value, nil
}

func managerProperty(ctx context.Context, bus managerBus, path dbus.ObjectPath, iface, property string, destination any) error {
	if err := validateManagerObjectPath("unit", path, systemdUnitPathPrefix); err != nil {
		return err
	}
	body, err := bus.call(ctx, systemdBusDestination, path, dbusPropertiesGet, iface, property)
	if err != nil {
		return fmt.Errorf("read fixed systemd %s.%s property: %w", iface, property, managerMethodError("Properties.Get", "", err))
	}
	if err := storeManagerReply(body, destination); err != nil {
		return fmt.Errorf("systemd manager returned an invalid %s.%s property: %w", iface, property, err)
	}
	return nil
}

func storeManagerReply(body []any, destinations ...any) error {
	if len(body) != len(destinations) {
		return fmt.Errorf("reply has %d values; expected %d", len(body), len(destinations))
	}
	if len(destinations) == 0 {
		return nil
	}
	return dbus.Store(body, destinations...)
}

func validateManagerUnits(units []string, allowEmpty bool) error {
	if len(units) == 0 && !allowEmpty {
		return errors.New("at least one systemd unit is required")
	}
	if len(units) > builtins.MaxSystemServiceOperands {
		return fmt.Errorf("systemd manager request has too many units (maximum %d)", builtins.MaxSystemServiceOperands)
	}
	seen := make(map[string]struct{}, len(units))
	for _, unit := range units {
		if err := validateManagerUnit(unit); err != nil {
			return err
		}
		if _, exists := seen[unit]; exists {
			return fmt.Errorf("systemd manager request contains duplicate unit %q", unit)
		}
		seen[unit] = struct{}{}
	}
	return nil
}

func validateManagerUnit(unit string) error {
	if unit == "" {
		return errors.New("systemd unit name must not be empty")
	}
	if len(unit) > builtins.MaxSystemServiceNameBytes {
		return fmt.Errorf("systemd unit name exceeds %d bytes", builtins.MaxSystemServiceNameBytes)
	}
	if !utf8.ValidString(unit) {
		return errors.New("systemd unit name must be valid UTF-8")
	}
	separator := strings.LastIndexByte(unit, '.')
	if separator <= 0 || separator == len(unit)-1 || !builtins.IsSupportedSystemdUnitType(unit[separator+1:]) {
		return fmt.Errorf("systemd unit name %q must have a supported unit suffix", unit)
	}
	base := unit[:separator]
	if strings.Count(base, "@") > 1 || strings.HasPrefix(base, "@") {
		return fmt.Errorf("systemd unit name %q has an invalid instance separator", unit)
	}
	for _, character := range base {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '_', '-', '.', '@':
			continue
		}
		return fmt.Errorf("systemd unit name %q contains an unsupported character", unit)
	}
	return nil
}

func validateManagerCanonicalUnit(unit string) error {
	if unit == "" {
		return errors.New("systemd unit name must not be empty")
	}
	if len(unit) > builtins.MaxSystemServiceNameBytes {
		return fmt.Errorf("systemd unit name exceeds %d bytes", builtins.MaxSystemServiceNameBytes)
	}
	if !utf8.ValidString(unit) {
		return errors.New("systemd unit name must be valid UTF-8")
	}
	separator := strings.LastIndexByte(unit, '.')
	if separator <= 0 || separator == len(unit)-1 || !builtins.IsSupportedSystemdUnitType(unit[separator+1:]) {
		return fmt.Errorf("systemd unit name %q must have a supported unit suffix", unit)
	}
	base := unit[:separator]
	if strings.Count(base, "@") > 1 || strings.HasPrefix(base, "@") {
		return fmt.Errorf("systemd unit name %q has an invalid instance separator", unit)
	}
	for index := 0; index < len(base); index++ {
		character := base[index]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '_', '-', '.', '@', ':':
			continue
		case '\\':
			if index+3 >= len(base) || base[index+1] != 'x' || !isASCIIHex(base[index+2]) || !isASCIIHex(base[index+3]) {
				return fmt.Errorf("systemd unit name %q contains an invalid escape", unit)
			}
			index += 3
			continue
		default:
			return fmt.Errorf("systemd unit name %q contains an unsupported character", unit)
		}
	}
	return nil
}

func isASCIIHex(character byte) bool {
	return character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F'
}

func validateManagerString(name, value string, allowEmpty bool) error {
	if value == "" && !allowEmpty {
		return fmt.Errorf("%s must not be empty", name)
	}
	if len(value) > builtins.MaxSystemServiceFieldBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, builtins.MaxSystemServiceFieldBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}

func validateManagerObjectPath(name string, path dbus.ObjectPath, prefix string) error {
	if len(path) > maxManagerObjectPath {
		return fmt.Errorf("systemd manager returned an oversized %s object path", name)
	}
	suffix, hasPrefix := strings.CutPrefix(string(path), prefix)
	if !path.IsValid() || !hasPrefix || suffix == "" || strings.ContainsRune(suffix, '/') {
		return fmt.Errorf("systemd manager returned an invalid %s object path", name)
	}
	return nil
}

func validateManagerJobPath(path dbus.ObjectPath, id uint32) error {
	if err := validateManagerObjectPath("job", path, systemdJobPathPrefix); err != nil {
		return err
	}
	parsed, err := strconv.ParseUint(strings.TrimPrefix(string(path), systemdJobPathPrefix), 10, 32)
	if err != nil || uint32(parsed) != id || id == 0 {
		return errors.New("systemd manager returned an invalid job object path")
	}
	return nil
}

func validateReturnedManagerJobPath(path dbus.ObjectPath) error {
	if err := validateManagerObjectPath("job", path, systemdJobPathPrefix); err != nil {
		return err
	}
	parsed, err := strconv.ParseUint(strings.TrimPrefix(string(path), systemdJobPathPrefix), 10, 32)
	if err != nil || parsed == 0 {
		return errors.New("systemd manager returned an invalid job object path")
	}
	return nil
}

func validateUnitFileChanges(changes []unitFileChange) error {
	if len(changes) > maxUnitFileChanges {
		return fmt.Errorf("systemd manager returned too many unit-file changes (maximum %d)", maxUnitFileChanges)
	}
	seen := make(map[unitFileChange]struct{}, len(changes))
	for _, change := range changes {
		if change.Type != "symlink" && change.Type != "unlink" {
			return errors.New("systemd manager returned an unsupported unit-file change type")
		}
		if err := validateManagerString("unit-file change destination", change.Destination, false); err != nil {
			return err
		}
		if err := validateManagerString("unit-file change source", change.Source, change.Type == "unlink"); err != nil {
			return err
		}
		if _, exists := seen[change]; exists {
			return errors.New("systemd manager returned a duplicate unit-file change")
		}
		seen[change] = struct{}{}
	}
	return nil
}

func isNoSuchUnitError(err error) bool {
	switch managerDBusErrorName(err) {
	case "org.freedesktop.systemd1.NoSuchUnit", "org.freedesktop.systemd1.NoSuchUnitFile":
		return true
	default:
		return false
	}
}

func managerDBusErrorName(err error) string {
	var value dbus.Error
	if errors.As(err, &value) {
		return value.Name
	}
	var pointer *dbus.Error
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Name
	}
	return ""
}

func managerMethodError(method, unit string, err error) error {
	if err == nil {
		return nil
	}
	if name := managerDBusErrorName(err); name != "" {
		if unit != "" {
			return fmt.Errorf("systemd manager %s failed for %q: %s", method, unit, name)
		}
		return fmt.Errorf("systemd manager %s failed: %s", method, name)
	}
	if unit != "" {
		return fmt.Errorf("systemd manager %s failed for %q: %w", method, unit, err)
	}
	return fmt.Errorf("systemd manager %s failed: %w", method, err)
}
