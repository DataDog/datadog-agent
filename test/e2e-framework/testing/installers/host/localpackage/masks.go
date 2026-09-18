// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var maskIdentityPattern = regexp.MustCompile(`^[0-9]+:[0-9]+$`)

func inspectServiceMask(remote Transport, service string) error {
	out, err := remote.Execute("systemctl show --all --property=LoadState --property=UnitFileState -- " + service)
	if err != nil {
		return fmt.Errorf("inspecting %s mask policy: %w", service, err)
	}
	properties := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "LoadState" && key != "UnitFileState") {
			return fmt.Errorf("unexpected systemctl inspection for %s", service)
		}
		if _, exists := properties[key]; exists {
			return fmt.Errorf("duplicate systemctl property for %s", service)
		}
		properties[key] = value
	}
	if len(properties) != 2 {
		return fmt.Errorf("incomplete systemctl inspection for %s", service)
	}
	if properties["LoadState"] == "masked" || properties["UnitFileState"] == "masked" || properties["UnitFileState"] == "masked-runtime" {
		return fmt.Errorf("%s already has an operator-owned mask", service)
	}
	switch properties["LoadState"] {
	case "not-found":
		if properties["UnitFileState"] == "" {
			return nil
		}
	case "loaded":
		switch properties["UnitFileState"] {
		case "enabled", "enabled-runtime", "disabled", "static", "indirect", "generated", "transient", "linked", "linked-runtime", "alias":
			return nil
		}
	}
	return fmt.Errorf("unsupported systemctl state for %s", service)
}

type ownedMask struct{ service, anchor, identity string }

// acquireServiceMasks creates exclusive hard links to private symlink anchors.
// Anchors keep inode identities alive, preventing a removed/replaced mask from
// looking owned through inode reuse. Existing masks are never adopted.
func acquireServiceMasks(remote Transport, nonce string) (func() error, error) {
	for _, service := range strings.Fields(services) {
		if err := inspectServiceMask(remote, service); err != nil {
			return nil, err
		}
	}
	dir := "/run/e2ectl-masks-" + nonce
	if _, err := remote.Execute("sudo mkdir -m 0700 -- " + dir); err != nil {
		return nil, err
	}
	owned := []ownedMask{}
	for _, service := range strings.Fields(services) {
		anchor := dir + "/" + service
		path := "/run/systemd/system/" + service
		out, err := remote.Execute("sudo sh -ec 'ln -s -- /dev/null " + anchor + "; ln -P -- " + anchor + " " + path + "; stat -c \"%d:%i\" -- " + path + "'")
		if err != nil || !maskIdentityPattern.MatchString(strings.TrimSpace(out)) {
			if err == nil {
				err = fmt.Errorf("invalid mask inode evidence")
			}
			_, reloadErr := remote.Execute("sudo systemctl daemon-reload")
			return nil, errors.Join(fmt.Errorf("creating exclusively owned mask for %s failed; retain %s for repair: %w", service, dir, err), reloadErr)
		}
		owned = append(owned, ownedMask{service: service, anchor: anchor, identity: strings.TrimSpace(out)})
	}
	if _, err := remote.Execute("sudo systemctl daemon-reload"); err != nil {
		return nil, err
	}
	return func() error {
		var failures []error
		for _, mask := range owned {
			path := "/run/systemd/system/" + mask.service
			command := "sudo sh -ec 'test \"$(stat -c %d:%i -- " + path + ")\" = " + mask.identity + " && test \"$(stat -c %d:%i -- " + mask.anchor + ")\" = " + mask.identity + " && test \"$(readlink -- " + path + ")\" = /dev/null && test \"$(readlink -- " + mask.anchor + ")\" = /dev/null && rm -- " + path + " " + mask.anchor + "'"
			if _, err := remote.Execute(command); err != nil {
				failures = append(failures, fmt.Errorf("mask ownership changed for %s; refusing removal: %w", mask.service, err))
			}
		}
		if _, err := remote.Execute("sudo systemctl daemon-reload"); err != nil {
			failures = append(failures, err)
		}
		if len(failures) == 0 {
			if _, err := remote.Execute("sudo rmdir -- " + dir); err != nil {
				return err
			}
		}
		return errors.Join(failures...)
	}, nil
}
