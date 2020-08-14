// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"path"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

// openTables is the list of eBPF tables used by open's kProbes
var openTables = []string{
	"open_policy",
	"open_basename_approvers",
	"open_flags_approvers",
	"open_flags_discarders",
	"open_process_inode_approvers",
	"open_path_inode_discarders",
}

func openOnNewApprovers(probe *Probe, approvers rules.Approvers) error {
	stringValues := func(fvs rules.FilterValues) []string {
		var values []string
		for _, v := range fvs {
			values = append(values, v.Value.(string))
		}
		return values
	}

	intValues := func(fvs rules.FilterValues) []int {
		var values []int
		for _, v := range fvs {
			values = append(values, v.Value.(int))
		}
		return values
	}

	for field, values := range approvers {
		switch field {
		case "process.filename":
			if err := approveProcessFilenames(probe, "open_process_inode_approvers", stringValues(values)...); err != nil {
				return err
			}

		case "open.basename":
			if err := approveBasenames(probe, "open_basename_approvers", stringValues(values)...); err != nil {
				return err
			}

		case "open.filename":
			for _, value := range stringValues(values) {
				basename := path.Base(value)
				if err := approveBasename(probe, "open_basename_approvers", basename); err != nil {
					return err
				}
			}

		case "open.flags":
			if err := approveFlags(probe, "open_flags_approvers", intValues(values)...); err != nil {
				return err
			}

		default:
			return errors.New("field unknown")
		}
	}

	return nil
}

func openOnNewDiscarder(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
	field := discarder.Field

	switch field {
	case "open.flags":
		return discardFlags(probe, "open_flags_discarders", discarder.Value.(int))

	case "open.filename":
		fsEvent := event.Open
		table := "open_path_inode_discarders"

		isDiscarded, err := discardParentInode(probe, rs, "open", discarder.Value.(string), fsEvent.MountID, fsEvent.Inode, table)
		if !isDiscarded || err != nil {
			// not able to discard the parent then only discard the filename
			_, err = discardInode(probe, fsEvent.MountID, fsEvent.Inode, table)
		}

		return err
	}
	return &ErrDiscarderNotSupported{Field: field}
}
