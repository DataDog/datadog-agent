// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

var unlinkTables = []string{
	"unlink_policy",
	"unlink_inode_discarders",
}

func unlinkOnNewDiscarder(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
	field := discarder.Field

	switch field {
	case "process.filename":
		return discardProcessFilename(probe, "unlink_process_discarders", event)
	case "unlink.filename":
		value, err := event.GetFieldValue(field)
		if err != nil {
			return err
		}
		filename := value.(string)

		if filename == "" {
			return nil
		}

		if probe.IsInvalidDiscarder(field, filename) {
			return nil
		}

		fsEvent := event.Unlink
		table := "unlink_inode_discarders"

		_, err = discardParentInode(probe, rs, "unlink", "unlink.filename", value, fsEvent.MountID, fsEvent.Inode, table)
		return err
	}
	return &ErrDiscarderNotSupported{Field: field}
}
