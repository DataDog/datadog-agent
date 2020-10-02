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
	"unlink_path_inode_discarders",
}

func unlinkOnNewDiscarder(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
	field := discarder.Field

	switch field {
	case "unlink.filename":
		fsEvent := event.Unlink
		table := "unlink_path_inode_discarders"
		value := discarder.Value.(string)

		if value == "" {
			return nil
		}

		_, err := discardParentInode(probe, rs, "unlink", value, fsEvent.MountID, fsEvent.Inode, table)
		return err
	}
	return &ErrDiscarderNotSupported{Field: field}
}
