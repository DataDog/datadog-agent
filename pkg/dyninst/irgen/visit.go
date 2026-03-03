// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"debug/dwarf"
	"errors"
	"fmt"
)

type visitErr struct {
	entry *dwarf.Entry
	cause error
}

func (e *visitErr) Error() string {
	return fmt.Sprintf("%s@0x%x: %v", e.entry.Tag, e.entry.Offset, e.cause)
}

func (e *visitErr) Unwrap() error {
	return e.cause
}

// Visit the DWARF reader, calling the visitor for each compile unit.
func visitDwarf(reader *dwarf.Reader, visitor visitor) (retErr error) {
	for {
		entry, err := reader.Next()
		if err != nil {
			return err
		}
		if entry == nil {
			break
		}
		if err := visitReader(entry, reader, visitor); err != nil {
			return err
		}
	}
	return nil
}

// Visit the current entry, and if it has children and the visitor has returned
// a child visitor, visit the children and then call pop with the visitor.
func visitReader(
	entry *dwarf.Entry,
	reader *dwarf.Reader,
	visitor visitor,
) (retErr error) {
	defer func() {
		if retErr == nil {
			return
		}
		retErr = &visitErr{
			entry: entry,
			cause: retErr,
		}
	}()
	childVisitor, err := visitor.push(entry)
	if err != nil {
		return err
	}
	if entry.Children && childVisitor == nil {
		reader.SkipChildren()
	} else if entry.Children {
		for {
			child, err := reader.Next()
			if err != nil {
				return fmt.Errorf(
					"visitReader: failed to get DWARF child entry: %w", err,
				)
			}
			if child == nil {
				return errors.New(
					"visitReader: unexpected EOF while reading children",
				)
			}
			if child.Tag == 0 {
				break
			}
			if err := visitReader(child, reader, childVisitor); err != nil {
				return err
			}
		}
	}
	return visitor.pop(entry, childVisitor)
}

type visitor interface {
	push(entry *dwarf.Entry) (childVisitor visitor, err error)
	pop(entry *dwarf.Entry, childVisitor visitor) error
}
