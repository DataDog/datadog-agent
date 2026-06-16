// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package listeners

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ncmConfigDirName is the conf.d sub-folder (integration name + ".d") that the NCM
	// check's file autodiscovery provider scans.
	ncmConfigDirName = "network_config_management.d"
	// ncmConfigFileName is the agent-generated NCM config file name. Using "auto_conf.yaml"
	// follows the convention for agent-generated autodiscovery configs (e.g. snmp.d/auto_conf.yaml).
	ncmConfigFileName = "auto_conf.yaml"
	// defaultNCMNamespace is used when no namespace is configured on the SNMP listener.
	defaultNCMNamespace = "default"
)

// ncmFileConfig is the top-level structure of the generated NCM config file. It mirrors the
// subset of the NCM check's config format (init_config + instances) needed to schedule checks.
type ncmFileConfig struct {
	InitConfig ncmInitConfig `yaml:"init_config"`
	Instances  []ncmInstance `yaml:"instances"`
}

// ncmInitConfig holds the global init_config block of the generated NCM config file. SSH is the
// global default applied to devices whose credential does not override it; it is omitted when no
// global SSH settings were configured.
type ncmInitConfig struct {
	Namespace string        `yaml:"namespace"`
	SSH       *ncmSSHConfig `yaml:"ssh,omitempty"`
}

// ncmSSHConfig holds the SSH settings written under init_config.ssh or instances[].auth.ssh. Every
// field is omitempty so only the settings the user actually specified are emitted (strict
// pass-through); NCM and the underlying SSH library apply their own defaults for omitted fields.
type ncmSSHConfig struct {
	Timeout               int      `yaml:"timeout,omitempty"`
	KnownHostsPath        string   `yaml:"known_hosts_path,omitempty"`
	InsecureSkipVerify    bool     `yaml:"insecure_skip_verify,omitempty"`
	Ciphers               []string `yaml:"ciphers,omitempty"`
	KeyExchanges          []string `yaml:"key_exchanges,omitempty"`
	HostKeyAlgorithms     []string `yaml:"host_key_algorithms,omitempty"`
	AllowLegacyAlgorithms bool     `yaml:"allow_legacy_algorithms,omitempty"`
}

// ncmInstance is a single discovered device entry under instances.
type ncmInstance struct {
	IPAddress string  `yaml:"ip_address"`
	Auth      ncmAuth `yaml:"auth"`
}

// ncmAuth holds the SSH credentials for a device instance. SSH, when set, fully replaces the
// global init_config.ssh on the NCM side, so the writer fills it with the merged (effective)
// settings; it is omitted when the credential has no SSH override (the device then inherits
// init_config.ssh).
type ncmAuth struct {
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	SSH      *ncmSSHConfig `yaml:"ssh,omitempty"`
}

// ncmDeviceEntry is the per-device NCM state tracked in SNMPListener.ncmDevices. It mirrors the
// role of snmpSubnet.deviceCache: the unit of state that is added/removed device-by-device.
type ncmDeviceEntry struct {
	ip    string
	creds []snmp.NCMCredential
}

// equal reports whether two device entries describe the same device with the same credentials.
// snmp.NCMCredential now embeds a pointer (SSH) and slices, so it is no longer comparable with
// ==; reflect.DeepEqual handles the nested SSH config (pointers and slices) correctly.
func (e ncmDeviceEntry) equal(o ncmDeviceEntry) bool {
	return e.ip == o.ip && reflect.DeepEqual(e.creds, o.creds)
}

// ncmConfigEnabled returns true if at least one subnet has NCM credentials configured.
// When this is false the NCM device tracking and file writing are never exercised.
func (l *SNMPListener) ncmConfigEnabled() bool {
	for _, config := range l.config.Configs {
		if len(config.NCM) > 0 {
			return true
		}
	}
	return false
}

// recordNCMDevice records (or updates) a discovered NCM-eligible device, mirroring how SNMP adds
// an entry to snmpSubnet.devices on registration. When the device is new or its credentials
// changed and write is true, the NCM config file is regenerated. The write flag is false when the
// device is being re-added from the persistent cache on startup (mirroring SNMP not rewriting its
// cache for cache-loaded devices); the single startup write in initializeSubnets covers those.
//
// It is safe to call while holding the listener lock: it only takes ncmWriteMu and never the
// listener RWMutex, so the lock order is always (listener lock -> ncmWriteMu).
func (l *SNMPListener) recordNCMDevice(entityID, deviceIP string, creds []snmp.NCMCredential, write bool) {
	if len(creds) == 0 {
		return
	}

	l.ncmWriteMu.Lock()
	defer l.ncmWriteMu.Unlock()

	if l.ncmDevices == nil {
		l.ncmDevices = map[string]ncmDeviceEntry{}
	}

	entry := ncmDeviceEntry{ip: deviceIP, creds: creds}
	if existing, ok := l.ncmDevices[entityID]; ok && existing.equal(entry) {
		// Device already advertised with identical credentials: nothing to do (per-device skip).
		log.Debugf("NCM device %s already present, skipping NCM config write", deviceIP)
		return
	}

	l.ncmDevices[entityID] = entry
	log.Debugf("Recorded NCM device %s (%d credential(s))", deviceIP, len(creds))

	if write {
		l.writeNCMConfigLocked()
	}
}

// removeNCMDevice drops a device from the NCM config file, mirroring how SNMP deletes an entry
// from snmpSubnet.devices once the device has been evicted (after AllowedFailures). The file is
// regenerated only when the device was actually being advertised.
func (l *SNMPListener) removeNCMDevice(entityID string) {
	l.ncmWriteMu.Lock()
	defer l.ncmWriteMu.Unlock()

	entry, ok := l.ncmDevices[entityID]
	if !ok {
		return
	}

	delete(l.ncmDevices, entityID)
	log.Debugf("Removed NCM device %s from NCM config", entry.ip)
	l.writeNCMConfigLocked()
}

// writeNCMConfig regenerates the NCM config file from the currently tracked devices. It is used
// for the one-shot startup write (after cache load); per-device updates go through
// recordNCMDevice / removeNCMDevice.
func (l *SNMPListener) writeNCMConfig() {
	l.ncmWriteMu.Lock()
	defer l.ncmWriteMu.Unlock()
	l.writeNCMConfigLocked()
}

// writeNCMConfigLocked (re)generates the aggregated NCM config file from l.ncmDevices. The caller
// must hold ncmWriteMu. When no devices remain, any existing file is removed rather than leaving
// an instance-less (invalid) NCM config.
func (l *SNMPListener) writeNCMConfigLocked() {
	confdPath := l.config.ConfdPath
	if confdPath == "" {
		log.Errorf("Couldn't write NCM autodiscovery config: confd_path is not set")
		return
	}
	dir := filepath.Join(confdPath, ncmConfigDirName)
	dest := filepath.Join(dir, ncmConfigFileName)

	instances := l.buildNCMInstancesLocked()

	if len(instances) == 0 {
		// Nothing to advertise: remove any stale file rather than writing an
		// instance-less config (which the file provider would reject).
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			log.Errorf("Couldn't remove stale NCM autodiscovery config %s: %s", dest, err)
			return
		}
		log.Infof("Removed NCM autodiscovery config (no devices) at %s", dest)
		return
	}

	fileConfig := ncmFileConfig{
		InitConfig: ncmInitConfig{
			Namespace: l.ncmNamespace(),
			// Strict pass-through of the global SSH defaults (if any). Nothing is injected:
			// if no host verification is configured anywhere, NCM will reject the device.
			SSH: toNCMSSH(l.config.InitConfig.SSH),
		},
		Instances: instances,
	}

	data, err := yaml.Marshal(&fileConfig)
	if err != nil {
		log.Errorf("Couldn't marshal NCM autodiscovery config: %s", err)
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Errorf("Couldn't create NCM autodiscovery config directory %s: %s", dir, err)
		return
	}

	if err := atomicWriteFile(dest, data); err != nil {
		log.Errorf("Couldn't write NCM autodiscovery config to %s: %s", dest, err)
		return
	}

	log.Infof("Wrote NCM autodiscovery config with %d instance(s) to %s", len(instances), dest)
}

// buildNCMInstancesLocked returns one instance per (tracked device IP x NCM credential). The
// caller must hold ncmWriteMu.
func (l *SNMPListener) buildNCMInstancesLocked() []ncmInstance {
	instances := make([]ncmInstance, 0, len(l.ncmDevices))
	for _, dev := range l.ncmDevices {
		for _, cred := range dev.creds {
			auth := ncmAuth{
				Username: cred.User,
				Password: cred.Password,
			}
			// A credential-level SSH block fully replaces init_config.ssh on the NCM side, so
			// write the merged (effective) block. Without an override, leave auth.ssh empty so
			// the device inherits the generated init_config.ssh.
			if cred.SSH != nil {
				auth.SSH = mergeNCMSSH(l.config.InitConfig.SSH, cred.SSH)
			}
			instances = append(instances, ncmInstance{
				IPAddress: dev.ip,
				Auth:      auth,
			})
		}
	}

	// Sort deterministically so the generated file is stable across runs.
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].IPAddress != instances[j].IPAddress {
			return instances[i].IPAddress < instances[j].IPAddress
		}
		if instances[i].Auth.Username != instances[j].Auth.Username {
			return instances[i].Auth.Username < instances[j].Auth.Username
		}
		return instances[i].Auth.Password < instances[j].Auth.Password
	})

	return instances
}

// toNCMSSH converts a snmp.NCMSSHConfig (config representation) into the writer's ncmSSHConfig,
// copying each field as-is (strict pass-through). It returns nil when src is nil so the caller can
// omit the SSH block entirely.
func toNCMSSH(src *snmp.NCMSSHConfig) *ncmSSHConfig {
	if src == nil {
		return nil
	}
	return &ncmSSHConfig{
		Timeout:               src.Timeout,
		KnownHostsPath:        src.KnownHostsPath,
		InsecureSkipVerify:    src.InsecureSkipVerify,
		Ciphers:               src.Ciphers,
		KeyExchanges:          src.KeyExchanges,
		HostKeyAlgorithms:     src.HostKeyAlgorithms,
		AllowLegacyAlgorithms: src.AllowLegacyAlgorithms,
	}
}

// mergeNCMSSH builds the effective SSH block for a credential. For each field, the override
// (credential-level ncm[].ssh) wins when it is set; otherwise the base (global init_config.ssh)
// value is kept. A field is considered "set" when it holds a non-zero value (non-zero timeout,
// true bool, non-empty string/slice). As a consequence an override can only turn a boolean on or
// set a non-zero timeout, never force a global value back to its zero value. override is expected
// to be non-nil (only called for overrides).
func mergeNCMSSH(base, override *snmp.NCMSSHConfig) *ncmSSHConfig {
	merged := toNCMSSH(base)
	if merged == nil {
		merged = &ncmSSHConfig{}
	}
	if override.Timeout != 0 {
		merged.Timeout = override.Timeout
	}
	if override.KnownHostsPath != "" {
		merged.KnownHostsPath = override.KnownHostsPath
	}
	if override.InsecureSkipVerify {
		merged.InsecureSkipVerify = override.InsecureSkipVerify
	}
	if len(override.Ciphers) > 0 {
		merged.Ciphers = override.Ciphers
	}
	if len(override.KeyExchanges) > 0 {
		merged.KeyExchanges = override.KeyExchanges
	}
	if len(override.HostKeyAlgorithms) > 0 {
		merged.HostKeyAlgorithms = override.HostKeyAlgorithms
	}
	if override.AllowLegacyAlgorithms {
		merged.AllowLegacyAlgorithms = override.AllowLegacyAlgorithms
	}
	return merged
}

// ncmNamespace returns the namespace to write into the generated NCM init_config.
func (l *SNMPListener) ncmNamespace() string {
	if l.config.Namespace != "" {
		return l.config.Namespace
	}
	return defaultNCMNamespace
}

// atomicWriteFile writes data to a temp file in the destination directory and renames it over
// dest, so readers (the file AD provider) never observe a partially written file.
func atomicWriteFile(dest string, data []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, filepath.Base(dest)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
