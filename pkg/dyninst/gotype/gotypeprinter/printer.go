// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package gotypeprinter produces YAML snapshots of Go types from runtime
// metadata.
package gotypeprinter

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
)

// WalkTypes traverses reachable types starting from the provided IDs using the
// given table.  Returns the parsed types and any non-fatal errors encountered.
func WalkTypes(
	table *gotype.Table, rootTypeIDs []gotype.TypeID,
) ([]gotype.Type, error) {
	toVisit := make([]gotype.TypeID, 0, len(rootTypeIDs))
	toVisit = append(toVisit, rootTypeIDs...)
	seen := make(map[gotype.TypeID]struct{}, len(toVisit))
	for _, id := range toVisit {
		seen[id] = struct{}{}
	}
	var all []gotype.Type
	var errs []error
	maybeAddType := func(id gotype.TypeID) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			toVisit = append(toVisit, id)
		}
	}
	var references []gotype.TypeID
	for len(toVisit) > 0 {
		var id gotype.TypeID
		id, toVisit = toVisit[len(toVisit)-1], toVisit[:len(toVisit)-1]
		gt, err := table.ParseGoType(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		references = gt.AppendReferences(references[:0])
		for _, r := range references {
			maybeAddType(r)
		}
		all = append(all, gt)
	}
	return all, errors.Join(errs...)
}

// CompareType compares two types by package path, name, and ID.
func CompareType(a, b gotype.Type) int {
	an := a.Name().UnsafeName()
	bn := b.Name().UnsafeName()
	return cmp.Or(
		cmp.Compare(a.PkgPath().UnsafeName(), b.PkgPath().UnsafeName()),
		cmp.Compare(an, bn),
		cmp.Compare(a.ID(), b.ID()),
	)
}

// TypeToYAML marshals a single type into YAML bytes using table for resolution.
func TypeToYAML(table *gotype.Table, typeID gotype.TypeID) ([]byte, error) {
	t, err := table.ParseGoType(typeID)
	if err != nil {
		return nil, err
	}
	entry, errs := buildEntryForTypeFromTable(t, table)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal([]typeEntry{entry})
	if err != nil {
		return nil, fmt.Errorf("marshal type %#x: %w", typeID, err)
	}
	return data, nil
}

// TypesToYAML marshals multiple types into YAML using the table for resolution.
func TypesToYAML(table *gotype.Table, types []gotype.Type) ([]byte, error) {
	entries := make([]typeEntry, 0, len(types))
	var errs []error
	for _, t := range types {
		entry, es := buildEntryForTypeFromTable(t, table)
		if len(es) > 0 {
			errs = append(errs, es...)
		}
		entries = append(entries, entry)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal types: %w", err)
	}
	return data, nil
}

// Snapshot structures for YAML output.
type typeEntry struct {
	ID            string          `yaml:"offset"`
	Size          uint64          `yaml:"size"`
	Kind          string          `yaml:"kind"`
	NameEntry     typeNameEntry   `yaml:"nameEntry"`
	Name          string          `yaml:"name"`
	Pkg           string          `yaml:"pkg"`
	PtrToThis     string          `yaml:"ptrToThis,omitempty"`
	Methods       []methodEntry   `yaml:"methods,omitempty"`
	InterfaceMeth []iMethodEntry  `yaml:"interfaceMethods,omitempty"`
	Array         *arrayDetails   `yaml:"array,omitempty"`
	Chan          *chanDetails    `yaml:"chan,omitempty"`
	Func          *funcDetails    `yaml:"func,omitempty"`
	Map           *mapDetails     `yaml:"map,omitempty"`
	Pointer       *pointerDetails `yaml:"pointer,omitempty"`
	Slice         *sliceDetails   `yaml:"slice,omitempty"`
	Struct        *structDetails  `yaml:"struct,omitempty"`
}

type methodEntry struct {
	Name nameEntry `yaml:"name"`
	Type string    `yaml:"type"`
}

type iMethodEntry struct {
	Name nameEntry `yaml:"name"`
	Type string    `yaml:"type"`
}

type nameEntry gotype.Name
type typeNameEntry gotype.TypeName

func (ne nameEntry) MarshalYAML() (interface{}, error) {
	return marshalNameEntry((gotype.Name)(ne))
}

func (tne typeNameEntry) MarshalYAML() (interface{}, error) {
	return marshalNameEntry((gotype.TypeName)(tne))
}

type gotypeName interface {
	gotype.Name | gotype.TypeName
	HasTag() bool
	Tag() string
	IsEmbedded() bool
	IsExported() bool
	Name() string
}

func marshalNameEntry[N gotypeName](n N) (string, error) {
	var buf strings.Builder
	buf.WriteString(n.Name())
	if n.HasTag() {
		_, _ = fmt.Fprintf(&buf, " (tag: %s)", n.Tag())
	}
	if n.IsEmbedded() {
		buf.WriteString(" (embedded)")
	}
	if n.IsExported() {
		buf.WriteString(" (exported)")
	}
	return buf.String(), nil
}

type arrayDetails struct {
	Len  uint64 `yaml:"len"`
	Elem string `yaml:"elem"`
}

type chanDetails struct {
	Dir  string `yaml:"dir"`
	Elem string `yaml:"elem"`
}

type funcDetails struct {
	In       []string `yaml:"in"`
	Out      []string `yaml:"out"`
	Variadic bool     `yaml:"variadic"`
}

type mapDetails struct {
	Key    string `yaml:"key"`
	Elem   string `yaml:"elem"`
	Bucket string `yaml:"bucket"`
}

type pointerDetails struct {
	Elem string `yaml:"elem"`
}

type sliceDetails struct {
	Elem string `yaml:"elem"`
}

type structDetails struct {
	Fields []structFieldEntry `yaml:"fields"`
}

type structFieldEntry struct {
	Name   nameEntry `yaml:"name"`
	Type   string    `yaml:"type"`
	Offset uint64    `yaml:"offset"`
}

// buildEntryForTypeFromTable converts a single type into a snapshot entry using table.
func buildEntryForTypeFromTable(t gotype.Type, table *gotype.Table) (typeEntry, []error) {
	var errs []error
	name := t.Name()
	te := typeEntry{
		ID:        fmt.Sprintf("0x%08x", uint32(t.ID())),
		Size:      t.Size(),
		Kind:      reflect.Kind(t.Kind()).String(),
		NameEntry: typeNameEntry(name),
		Name:      name.Name(),
		Pkg:       t.PkgPath().Name(),
	}
	if ptr, ok := t.PtrToThis(); ok {
		te.PtrToThis = fmt.Sprintf("0x%08x", uint32(ptr))
	}
	typeName := func(id gotype.TypeID) string {
		if id == ^gotype.TypeID(0) {
			return "<unreachable>"
		}
		name, err := typeNameByID(table, id)
		if err != nil {
			err = fmt.Errorf("resolve type %#x for name: %w", id, err)
			errs = append(errs, err)
			return err.Error()
		}
		return name
	}
	if ms, err := t.Methods(nil); err == nil {
		for _, m := range ms {
			te.Methods = append(te.Methods, methodEntry{
				Name: nameEntry(m.Name.Resolve(table)),
				Type: typeName(m.Mtyp),
			})
		}
	} else {
		errs = append(errs, err)
	}
	if i, ok := t.Interface(); ok {
		if im, err := i.Methods(nil); err == nil && len(im) > 0 {
			for _, m := range im {
				te.InterfaceMeth = append(te.InterfaceMeth, iMethodEntry{
					Name: nameEntry(m.Name.Resolve(table)),
					Type: typeName(m.Typ),
				})
			}
		} else if err != nil {
			errs = append(errs, err)
		}
	}
	switch reflect.Kind(t.Kind()) {
	case reflect.Array:
		if a, ok := t.Array(); ok {
			if ln, elem, err := a.LenElem(); err == nil {
				te.Array = &arrayDetails{Len: ln, Elem: typeName(elem)}
			} else {
				errs = append(errs, err)
			}
		}
	case reflect.Chan:
		if c, ok := t.Chan(); ok {
			if dir, elem, err := c.DirElem(); err == nil {
				dirStr := "both"
				switch dir {
				case 1:
					dirStr = "recv"
				case 2:
					dirStr = "send"
				case 3:
					dirStr = "both"
				}
				te.Chan = &chanDetails{Dir: dirStr, Elem: typeName(elem)}
			} else {
				errs = append(errs, err)
			}
		}
	case reflect.Func:
		if f, ok := t.Func(); ok {
			if in, out, variadic, err := f.Signature(); err == nil {
				fd := funcDetails{Variadic: variadic}
				for _, p := range in {
					fd.In = append(fd.In, typeName(p))
				}
				for _, r := range out {
					fd.Out = append(fd.Out, typeName(r))
				}
				te.Func = &fd
			} else {
				errs = append(errs, err)
			}
		}
	case reflect.Map:
		if m, ok := t.Map(); ok {
			if k, v, err := m.KeyValue(); err == nil {
				te.Map = &mapDetails{Key: typeName(k), Elem: typeName(v)}
			} else {
				errs = append(errs, err)
			}
		}
	case reflect.Pointer:
		if p, ok := t.Pointer(); ok {
			e := p.Elem()
			te.Pointer = &pointerDetails{Elem: typeName(e)}
		}
	case reflect.Slice:
		if s, ok := t.Slice(); ok {
			e := s.Elem()
			te.Slice = &sliceDetails{Elem: typeName(e)}
		}
	case reflect.Struct:
		if s, ok := t.Struct(); ok {
			if fs, err := s.Fields(); err == nil {
				sd := structDetails{}
				for _, f := range fs {
					sd.Fields = append(sd.Fields, structFieldEntry{
						Name:   nameEntry(f.Name.Resolve(table)),
						Type:   typeName(f.Typ),
						Offset: f.Offset,
					})
				}
				te.Struct = &sd
			} else {
				errs = append(errs, err)
			}
		}
	}
	return te, errs
}

func typeName(ty gotype.Type, table *gotype.Table) (string, error) {
	name := ty.Name()
	if !name.IsEmpty() {
		return name.Name(), nil
	}
	kind := ty.Kind()
	switch kind {
	case reflect.Pointer:
		if ptr, ok := ty.Pointer(); ok {
			elem := ptr.Elem()
			elemName, err := typeNameByID(table, elem)
			if err != nil {
				return "", err
			}
			return "*" + elemName, nil
		}
	case reflect.Slice:
		if s, ok := ty.Slice(); ok {
			elem := s.Elem()
			elemName, err := typeNameByID(table, elem)
			if err != nil {
				return "", err
			}
			return "[]" + elemName, nil
		}
	case reflect.Map:
		if m, ok := ty.Map(); ok {
			key, elem, err := m.KeyValue()
			if err != nil {
				return "", err
			}
			keyName, err := typeNameByID(table, key)
			if err != nil {
				return "", err
			}
			elemName, err := typeNameByID(table, elem)
			if err != nil {
				return "", err
			}
			return "map[" + keyName + "]" + elemName, nil
		}
	case reflect.Func:
		if f, ok := ty.Func(); ok {
			in, out, variadic, err := f.Signature()
			if err != nil {
				return "", err
			}
			inNames := make([]string, 0, len(in))
			for _, i := range in {
				inName, err := typeNameByID(table, i)
				if err != nil {
					return "", err
				}
				inNames = append(inNames, inName)
			}
			if variadic {
				inNames = append(inNames, "...")
			}
			outNames := make([]string, 0, len(out))
			for _, o := range out {
				outName, err := typeNameByID(table, o)
				if err != nil {
					return "", err
				}
				outNames = append(outNames, outName)
			}
			return "func(" + strings.Join(inNames, ", ") + ")" + strings.Join(outNames, ", "), nil
		}
	case reflect.Chan:
		if c, ok := ty.Chan(); ok {
			dir, elem, err := c.DirElem()
			if err != nil {
				return "", err
			}
			dirStr := "chan"
			switch dir {
			case 1:
				dirStr = "<-chan "
			case 2:
				dirStr = "chan<- "
			}
			elemName, err := typeNameByID(table, elem)
			if err != nil {
				return "", err
			}
			return dirStr + elemName, nil
		}
	case reflect.Interface:
		if i, ok := ty.Interface(); ok {
			im, err := i.Methods(nil)
			if err != nil {
				return "", err
			}
			if len(im) > 0 {
				return "interface{...}", nil
			}
		}
		return "interface{}", nil
	case reflect.Struct:
		if s, ok := ty.Struct(); ok {
			fs, err := s.Fields()
			if err != nil {
				return "", err
			}
			if len(fs) > 0 {
				return "struct{...}", nil
			}
		}
		return "struct{}", nil
	case reflect.Array:
		if a, ok := ty.Array(); ok {
			ln, elem, err := a.LenElem()
			if err != nil {
				return "", err
			}
			elemName, err := typeNameByID(table, elem)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("[%d]%s", ln, elemName), nil
		}
		return "array{}", nil
	}
	return fmt.Sprintf("<0x%x (%v)>", ty.ID(), kind), nil
}

// typeNameByID resolves a type ID to a name using the table.
func typeNameByID(table *gotype.Table, typeID gotype.TypeID) (string, error) {
	t, err := table.ParseGoType(typeID)
	if err != nil {
		return "", fmt.Errorf("parse type %#x for name: %w", typeID, err)
	}
	return typeName(t, table)
}
