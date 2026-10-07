// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ioscmd parses a JSON description of a block of Cisco IOS commands
// and renders it into the CLI lines to send to a device.
//
// The JSON format is defined by the TypeScript types in
// ios_commands.ts in this package.
package ioscmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// CommandBlock is an ordered list of top-level commands.
type CommandBlock []Command

// Command is a top-level command: a ShowCommand, *HostnameCommand, or
// *InterfaceCommand.
type Command interface {
	// isConfig reports whether the command must be run in global configuration mode.
	isConfig() bool
	// render returns the CLI lines for the command, or an error if it is invalid.
	render() ([]string, error)
}

// Parse decodes and validates a JSON command block.
func Parse(data []byte) (CommandBlock, error) {
	var b CommandBlock
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

// Generate parses a JSON command block and renders it into CLI lines.
func Generate(data []byte) ([]string, error) {
	b, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return b.Render()
}

// Validate returns an error if any command in the block is invalid.
func (b CommandBlock) Validate() error {
	_, err := b.Render()
	return err
}

// Render converts the block into the CLI lines to send to the device. It
// emits `configure terminal` before each run of configuration commands and
// `end` after it, so show and configuration commands can be freely mixed.
func (b CommandBlock) Render() ([]string, error) {
	var out []string
	inConfig := false
	for i, cmd := range b {
		if cmd == nil {
			return nil, fmt.Errorf("command %d: nil command", i)
		}
		lines, err := cmd.render()
		if err != nil {
			return nil, fmt.Errorf("command %d: %w", i, err)
		}
		if cmd.isConfig() != inConfig {
			if inConfig {
				out = append(out, "end")
			} else {
				out = append(out, "configure terminal")
			}
			inConfig = !inConfig
		}
		out = append(out, lines...)
	}
	if inConfig {
		out = append(out, "end")
	}
	return out, nil
}

// UnmarshalJSON decodes a JSON array of commands, dispatching on each
// command's "type" (and, for show commands, "target") field.
func (b *CommandBlock) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	if raws == nil {
		return errors.New("command block must be a JSON array")
	}
	cmds := make(CommandBlock, 0, len(raws))
	for i, raw := range raws {
		cmd, err := decodeCommand(raw)
		if err != nil {
			return fmt.Errorf("command %d: %w", i, err)
		}
		cmds = append(cmds, cmd)
	}
	*b = cmds
	return nil
}

// discriminator holds the fields used to pick the concrete type of a command.
type discriminator struct {
	Type   string `json:"type"`
	Target string `json:"target"`
}

func peek(raw json.RawMessage) (discriminator, error) {
	var d discriminator
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, err
	}
	if d.Type == "" {
		return d, errors.New(`missing "type"`)
	}
	return d, nil
}

func decodeCommand(raw json.RawMessage) (Command, error) {
	d, err := peek(raw)
	if err != nil {
		return nil, err
	}
	var cmd Command
	switch d.Type {
	case "show":
		newShow, ok := showTargets[d.Target]
		if !ok {
			return nil, fmt.Errorf("unknown show target %q", d.Target)
		}
		cmd = newShow()
	case "hostname":
		cmd = &HostnameCommand{}
	case "interface":
		cmd = &InterfaceCommand{}
	default:
		return nil, fmt.Errorf("unknown command type %q", d.Type)
	}
	if err := decodeStrict(raw, cmd); err != nil {
		return nil, fmt.Errorf("%s: %w", d.Type, err)
	}
	return cmd, nil
}

// decodeStrict decodes data into v, rejecting fields that v does not define.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Nullable is a JSON field that must be present but may be null. Null
// usually means "emit the `no` form of the command".
type Nullable[T any] struct {
	// Set is true if the field was present in the JSON.
	Set bool
	// Value is nil if the field was null.
	Value *T
}

// Null returns a Nullable that is set to null.
func Null[T any]() Nullable[T] {
	return Nullable[T]{Set: true}
}

// Some returns a Nullable that is set to v.
func Some[T any](v T) Nullable[T] {
	return Nullable[T]{Set: true, Value: &v}
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	n.Set = true
	n.Value = nil
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var v T
	if err := decodeStrict(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.Value)
}
