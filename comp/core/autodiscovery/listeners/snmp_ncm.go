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
	ncmConfigDirName    = "network_config_management.d"
	ncmConfigFileName   = "auto_conf.yaml"
	defaultNCMNamespace = "default"
)

// ncmFileConfig is the top-level structure of the generated NCM config file.
type ncmFileConfig struct {
	InitConfig ncmInitConfig `yaml:"init_config"`
	Instances  []ncmInstance `yaml:"instances"`
}

// ncmInitConfig holds the global init_config block of the generated NCM config file.
type ncmInitConfig struct {
	Namespace string        `yaml:"namespace"`
	SSH       *ncmSSHConfig `yaml:"ssh,omitempty"`
}

// ncmSSHConfig holds the SSH settings written under init_config.ssh or instances[].auth.ssh.
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

// ncmAuth holds the SSH credentials for a device instance.
type ncmAuth struct {
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	SSH      *ncmSSHConfig `yaml:"ssh,omitempty"`
}

// ncmDeviceEntry is the per-device NCM state tracked in SNMPListener.ncmDevices.
type ncmDeviceEntry struct {
	ip    string
	creds []snmp.NCMCredential
}

// equal reports whether two device entries describe the same device with the same credentials.
func (e ncmDeviceEntry) equal(o ncmDeviceEntry) bool {
	return e.ip == o.ip && reflect.DeepEqual(e.creds, o.creds)
}

func (l *SNMPListener) ncmConfigEnabled() bool {
	for _, config := range l.config.Configs {
		if len(config.NCM) > 0 {
			return true
		}
	}
	return false
}

// recordNCMDevice records (or updates) a discovered NCM-eligible device.
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

// removeNCMDevice drops a device from the NCM config file.
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

// writeNCMConfig regenerates the NCM config file from the currently tracked devices.
func (l *SNMPListener) writeNCMConfig() {
	l.ncmWriteMu.Lock()
	defer l.ncmWriteMu.Unlock()
	l.writeNCMConfigLocked()
}

// writeNCMConfigLocked (re)generates the aggregated NCM config file from l.ncmDevices.
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
			SSH:       toNCMSSH(l.config.InitConfig.SSH),
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

// buildNCMInstancesLocked returns one instance per (tracked device IP x NCM credential).
func (l *SNMPListener) buildNCMInstancesLocked() []ncmInstance {
	instances := make([]ncmInstance, 0, len(l.ncmDevices))
	for _, dev := range l.ncmDevices {
		for _, cred := range dev.creds {
			auth := ncmAuth{
				Username: cred.User,
				Password: cred.Password,
			}
			if cred.SSH != nil {
				auth.SSH = mergeNCMSSH(l.config.InitConfig.SSH, cred.SSH)
			}
			instances = append(instances, ncmInstance{
				IPAddress: dev.ip,
				Auth:      auth,
			})
		}
	}

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

// mergeNCMSSH builds the effective SSH block for a credential.
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

func (l *SNMPListener) ncmNamespace() string {
	if l.config.Namespace != "" {
		return l.config.Namespace
	}
	return defaultNCMNamespace
}

// atomicWriteFile writes data to a temp file in the destination directory and renames it over dest.
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
