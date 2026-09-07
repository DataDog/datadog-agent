// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configschema derives strict validation, defaults and examples from
// data-only Go types. It deliberately has no provider or runtime dependencies.
package configschema

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Validator is an optional semantic check, invoked after automatic validation.
// Implementations must be pure: discovery and generation also use these checks.
// Define shared rules beside the config type so both process boundaries use them.
type Validator[T any] interface {
	Validate(params T) error
}

// Schema is compiled once at registration and can be used concurrently. Only
// validator implementations with mutable state need their own synchronization.
type Schema[T any] struct {
	root       *shape
	validators []Validator[T]
}

type shape struct {
	typ    reflect.Type
	fields []field
	elem   *shape
}

type field struct {
	name        string
	shape       *shape
	required    bool
	secret      bool
	description string
	def         *yaml.Node
	example     *yaml.Node
	enum        []string
	pattern     *regexp.Regexp
	min, max    *int64
}

// Compile inspects yaml names and these annotations: config:"required,secret",
// description, default, example, enum (comma-separated), pattern, minimum and
// maximum. Bounds apply to integers. Null, aliases, duplicate and unknown keys
// are rejected. Unsupported schema types and invalid annotations fail here.
func Compile[T any](validators ...Validator[T]) (*Schema[T], error) {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config schema root must be a struct, got %v", t)
	}
	root, err := compileShape(t, make(map[reflect.Type]bool))
	if err != nil {
		return nil, err
	}
	for _, v := range validators {
		if v == nil || (reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()) {
			return nil, errors.New("nil config validator")
		}
	}
	return &Schema[T]{root: root, validators: append([]Validator[T](nil), validators...)}, nil
}

// Must is for static schema declarations. An invalid schema is a programming
// error; invalid user input is always returned as an error by Decode/DecodeNode.
func Must[T any](validators ...Validator[T]) *Schema[T] {
	s, err := Compile(validators...)
	if err != nil {
		panic(err)
	}
	return s
}

// ParseDocument reads one YAML document without losing source locations.
// An absent section is an empty mapping, so its field defaults/requiredness apply.
func ParseDocument(raw []byte) (*yaml.Node, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return mapping(), nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("expected a single YAML document")
	}
	if len(doc.Content) != 1 {
		return nil, errors.New("expected a YAML value")
	}
	return doc.Content[0], nil
}

// Decode returns the typed value and normalized YAML, including explicit zeros,
// false values and materialized defaults. Examples are NEVER runtime defaults.
func (s *Schema[T]) Decode(raw []byte, path string) (T, []byte, error) {
	n, err := ParseDocument(raw)
	if err != nil {
		var zero T
		return zero, nil, fmt.Errorf("%s: %w", path, err)
	}
	return s.DecodeNode(n, path)
}

// DecodeResolved validates a normalized process-boundary payload. Defaulted
// fields must already be explicit: a receiving binary must not choose defaults
// on behalf of a sender compiled with a different contract.
func (s *Schema[T]) DecodeResolved(raw []byte, path string) (T, error) {
	var zero T
	n, err := ParseDocument(raw)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.root.checkPolicy(n, path, resolvedInput); err != nil {
		return zero, err
	}
	value, _, err := s.DecodeNode(n, path)
	return value, err
}

// DecodeNode preserves the original node's positions in field errors. Input
// nodes are not mutated; callers may keep them for diagnostics or other schemas.
func (s *Schema[T]) DecodeNode(n *yaml.Node, path string) (T, []byte, error) {
	var value T
	if n == nil {
		n = mapping()
	}
	normal, errs := s.root.normalize(n, path)
	if err := errors.Join(errs...); err != nil {
		return value, nil, err
	}
	if err := normal.Decode(&value); err != nil {
		return value, nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, validator := range s.validators {
		if err := validator.Validate(value); err != nil {
			return value, nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	data, err := Encode(normal)
	return value, data, err
}

// Example generates annotated YAML from defaults/examples and explicit selector
// overrides (e.g. agent.install). It validates the resulting active document,
// including the optional semantic hooks. Optional fields with no example or
// default are omitted; required fields with neither need an override.
func (s *Schema[T]) Example(overrides map[string]any) (*yaml.Node, error) {
	n, err := s.root.example(overrides, "config")
	if err != nil {
		return nil, err
	}
	if err := s.root.checkPolicy(n, "example", generatedExample); err != nil {
		return nil, err
	}
	if _, _, err := s.DecodeNode(n, "example"); err != nil {
		return nil, fmt.Errorf("invalid generated example: %w", err)
	}
	return n, nil
}

// Encode renders YAML in a stable, human-readable layout.
func Encode(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	copy := cloneNode(n)
	blockStyle(copy)
	if err := enc.Encode(copy); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func compileShape(t reflect.Type, active map[reflect.Type]bool) (*shape, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	for _, iface := range []reflect.Type{
		reflect.TypeFor[yaml.Unmarshaler](), reflect.TypeFor[yaml.Marshaler](),
		reflect.TypeFor[encoding.TextUnmarshaler](), reflect.TypeFor[encoding.TextMarshaler](),
	} {
		if t.Implements(iface) || reflect.PointerTo(t).Implements(iface) {
			return nil, fmt.Errorf("config type %v uses a custom codec; explicit schema adapters are required", t)
		}
	}
	if active[t] {
		return nil, fmt.Errorf("recursive config type %v is not supported", t)
	}
	active[t] = true
	defer delete(active, t)
	s := &shape{typ: t}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	case reflect.Slice:
		var err error
		s.elem, err = compileShape(t.Elem(), active)
		if err != nil {
			return nil, err
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("config map %v must have string keys", t)
		}
		var err error
		s.elem, err = compileShape(t.Elem(), active)
		if err != nil {
			return nil, err
		}
	case reflect.Struct:
		seen := make(map[string]bool)
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if err := validateTags(sf.Tag); err != nil {
				return nil, fmt.Errorf("config field %v.%s: %w", t, sf.Name, err)
			}
			tag := strings.Split(sf.Tag.Get("yaml"), ",")
			if tag[0] == "-" {
				continue
			}
			if !sf.IsExported() {
				return nil, fmt.Errorf("config field %v.%s must be exported or marked yaml:\"-\"", t, sf.Name)
			}
			for _, opt := range tag[1:] {
				if opt != "omitempty" && opt != "" {
					return nil, fmt.Errorf("config field %v.%s: unsupported YAML option %q", t, sf.Name, opt)
				}
			}
			name := tag[0]
			if name == "" {
				name = strings.ToLower(sf.Name)
			}
			if seen[name] {
				return nil, fmt.Errorf("duplicate config field %q in %v", name, t)
			}
			seen[name] = true
			child, err := compileShape(sf.Type, active)
			if err != nil {
				return nil, fmt.Errorf("config field %s: %w", name, err)
			}
			f, err := compileField(sf, name, child)
			if err != nil {
				return nil, fmt.Errorf("config field %s: %w", name, err)
			}
			s.fields = append(s.fields, f)
		}
	default:
		return nil, fmt.Errorf("unsupported config type %v; use data-only fields", t)
	}
	return s, nil
}

// Schema metadata has a deliberately bounded vocabulary. Catch misspellings at
// compilation rather than silently ignoring a rule the config author intended.
func validateTags(tag reflect.StructTag) error {
	text := strings.TrimSpace(string(tag))
	seen := make(map[string]bool)
	for text != "" {
		colon := strings.IndexByte(text, ':')
		if colon <= 0 {
			return errors.New("invalid struct tag")
		}
		key := text[:colon]
		switch key {
		case "yaml", "json", "config", "description", "default", "example", "enum", "pattern", "minimum", "maximum":
		default:
			return fmt.Errorf("unknown schema annotation %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate schema annotation %q", key)
		}
		seen[key] = true
		text = text[colon+1:]
		if len(text) == 0 || text[0] != '"' {
			return errors.New("schema tag values must be quoted")
		}
		end := 1
		for end < len(text) && text[end] != '"' {
			if text[end] == '\\' {
				end++
			}
			end++
		}
		if end >= len(text) {
			return errors.New("unterminated schema tag")
		}
		if _, err := strconv.Unquote(text[:end+1]); err != nil {
			return err
		}
		text = text[end+1:]
		if text != "" && text[0] != ' ' {
			return errors.New("schema tags must be separated by spaces")
		}
		text = strings.TrimSpace(text)
	}
	return nil
}

func compileField(sf reflect.StructField, name string, child *shape) (field, error) {
	f := field{name: name, shape: child, description: sf.Tag.Get("description")}
	for _, flag := range strings.Split(sf.Tag.Get("config"), ",") {
		switch flag {
		case "":
		case "required":
			f.required = true
		case "secret":
			f.secret = true
		default:
			return f, fmt.Errorf("unknown config annotation %q", flag)
		}
	}
	if text, ok := sf.Tag.Lookup("enum"); ok {
		if child.typ.Kind() != reflect.String {
			return f, errors.New("enum requires a string field")
		}
		for _, value := range strings.Split(text, ",") {
			f.enum = append(f.enum, strings.TrimSpace(value))
		}
	}
	if text, ok := sf.Tag.Lookup("pattern"); ok {
		if child.typ.Kind() != reflect.String {
			return f, errors.New("pattern requires a string field")
		}
		var err error
		f.pattern, err = regexp.Compile(text)
		if err != nil {
			return f, err
		}
	}
	for name, target := range map[string]**int64{"minimum": &f.min, "maximum": &f.max} {
		if text, ok := sf.Tag.Lookup(name); ok {
			if child.typ.Kind() < reflect.Int || child.typ.Kind() > reflect.Int64 {
				return f, fmt.Errorf("%s requires a signed integer field", name)
			}
			v, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return f, fmt.Errorf("invalid %s: %w", name, err)
			}
			*target = &v
		}
	}
	if f.min != nil && f.max != nil && *f.min > *f.max {
		return f, errors.New("minimum exceeds maximum")
	}
	for name, target := range map[string]**yaml.Node{"default": &f.def, "example": &f.example} {
		if text, ok := sf.Tag.Lookup(name); ok {
			if f.secret {
				return f, errors.New("secret fields cannot declare defaults or examples")
			}
			var n *yaml.Node
			var err error
			if child.typ.Kind() == reflect.String {
				n = scalar("!!str", text)
			} else {
				n, err = ParseDocument([]byte(text))
			}
			if err != nil {
				return f, fmt.Errorf("invalid %s: %w", name, err)
			}
			normal, errs := child.normalize(n, f.name)
			if len(errs) == 0 {
				errs = f.constraints(normal, f.name)
			}
			if err := errors.Join(errs...); err != nil {
				return f, fmt.Errorf("invalid %s: %w", name, err)
			}
			if err := child.checkPolicy(normal, f.name, generatedExample); err != nil {
				return f, fmt.Errorf("invalid %s: %w", name, err)
			}
			*target = normal
		}
	}
	return f, nil
}

func (s *shape) normalize(n *yaml.Node, path string) (*yaml.Node, []error) {
	if n.Kind == yaml.AliasNode || n.Tag == "!!null" {
		return nil, []error{at(n, path, "aliases and null are not supported")}
	}
	out := *n
	out.HeadComment, out.LineComment, out.FootComment = "", "", ""
	out.Content = nil
	switch s.typ.Kind() {
	case reflect.Struct, reflect.Map:
		if n.Kind != yaml.MappingNode {
			return nil, []error{at(n, path, "expected a mapping")}
		}
		members, err := mappingMembers(n, path)
		if err != nil {
			return nil, []error{err}
		}
		var errs []error
		if s.typ.Kind() == reflect.Map {
			for i := 0; i < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				value, es := s.elem.normalize(v, path+"."+k.Value)
				errs = append(errs, es...)
				if value != nil {
					out.Content = append(out.Content, scalar("!!str", k.Value), value)
				}
			}
			return &out, errs
		}
		for _, f := range s.fields {
			value, present := members[f.name]
			delete(members, f.name)
			if !present {
				if f.def != nil {
					value = f.def
				} else {
					if f.required {
						errs = append(errs, at(n, path+"."+f.name, "required field is missing"))
					}
					continue
				}
			}
			normal, es := f.shape.normalize(value, path+"."+f.name)
			errs = append(errs, es...)
			if len(es) == 0 {
				errs = append(errs, f.constraints(normal, path+"."+f.name)...)
				out.Content = append(out.Content, scalar("!!str", f.name), normal)
			}
		}
		// Keep diagnostics in input order, not map iteration order.
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if _, ok := members[k.Value]; ok {
				errs = append(errs, at(k, path+"."+k.Value, "unknown field"))
			}
		}
		return &out, errs
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return nil, []error{at(n, path, "expected a sequence")}
		}
		var errs []error
		for i, v := range n.Content {
			value, es := s.elem.normalize(v, fmt.Sprintf("%s[%d]", path, i))
			errs = append(errs, es...)
			if value != nil {
				out.Content = append(out.Content, value)
			}
		}
		return &out, errs
	default:
		want := "!!int"
		if s.typ.Kind() == reflect.String {
			want = "!!str"
		} else if s.typ.Kind() == reflect.Bool {
			want = "!!bool"
		}
		if n.Kind != yaml.ScalarNode || n.Tag != want {
			return nil, []error{at(n, path, "expected %s", s.typ.Kind())}
		}
		// Validate range/overflow and custom named types with the actual Go type.
		value := reflect.New(s.typ)
		if err := n.Decode(value.Interface()); err != nil {
			return nil, []error{at(n, path, "cannot decode as %v", s.typ)}
		}
		return &out, nil
	}
}

func (f field) constraints(n *yaml.Node, path string) []error {
	var errs []error
	if len(f.enum) != 0 {
		found := false
		for _, v := range f.enum {
			found = found || n.Value == v
		}
		if !found {
			errs = append(errs, at(n, path, "must be one of: %s", strings.Join(f.enum, ", ")))
		}
	}
	if f.pattern != nil && !f.pattern.MatchString(n.Value) {
		errs = append(errs, at(n, path, "does not match the required format"))
	}
	if f.min != nil || f.max != nil {
		var value int64
		if err := n.Decode(&value); err != nil {
			return append(errs, at(n, path, "expected integer"))
		}
		if f.min != nil && value < *f.min {
			errs = append(errs, at(n, path, "must be at least %d", *f.min))
		}
		if f.max != nil && value > *f.max {
			errs = append(errs, at(n, path, "must be at most %d", *f.max))
		}
	}
	return errs
}

type inputPolicy int

const (
	generatedExample inputPolicy = iota
	resolvedInput
)

// Apply policies recursively, including through optional parent structs,
// slices and maps. Ordinary runtime decoding still permits explicit secrets;
// only generated defaults/examples/overrides must never contain them.
func (s *shape) checkPolicy(n *yaml.Node, path string, policy inputPolicy) error {
	switch s.typ.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return nil // normal validation will report the shape mismatch
		}
		members, err := mappingMembers(n, path)
		if err != nil {
			return err
		}
		for _, f := range s.fields {
			value, present := members[f.name]
			if !present {
				if policy == resolvedInput && f.def != nil {
					return at(n, path+"."+f.name, "normalized input must include its resolved default explicitly")
				}
				continue
			}
			if policy == generatedExample && f.secret {
				return at(value, path+"."+f.name, "secret fields cannot be emitted in defaults, examples or overrides")
			}
			if err := f.shape.checkPolicy(value, path+"."+f.name, policy); err != nil {
				return err
			}
		}
	case reflect.Map:
		if n.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if err := s.elem.checkPolicy(n.Content[i+1], path+"."+n.Content[i].Value, policy); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return nil
		}
		for i, value := range n.Content {
			if err := s.elem.checkPolicy(value, fmt.Sprintf("%s[%d]", path, i), policy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *shape) example(overrides map[string]any, path string) (*yaml.Node, error) {
	n := mapping()
	known := make(map[string]bool)
	for _, f := range s.fields {
		known[f.name] = true
		var value *yaml.Node
		source := ""
		if override, ok := overrides[f.name]; ok {
			value = &yaml.Node{}
			if f.secret {
				return nil, fmt.Errorf("%s.%s: cannot emit a secret example", path, f.name)
			}
			if err := value.Encode(override); err != nil {
				return nil, err
			}
		} else if f.def != nil {
			value, source = f.def, "Default value."
		} else if f.example != nil {
			value, source = f.example, "Example value; adjust before provisioning."
		} else if f.required {
			if f.shape.typ.Kind() == reflect.Struct && !f.secret {
				var err error
				value, err = f.shape.example(nil, path+"."+f.name)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("%s.%s: required field has no default, example or override", path, f.name)
			}
		} else {
			continue
		}
		k := scalar("!!str", f.name)
		k.HeadComment = strings.TrimSpace(f.description + "\n" + source)
		n.Content = append(n.Content, k, cloneNode(value))
	}
	for key := range overrides {
		if !known[key] {
			return nil, fmt.Errorf("%s.%s: unknown example override", path, key)
		}
	}
	return n, nil
}

// Mapping validates the keys of an envelope whose selected child schema is
// supplied by a registration (rather than a fixed struct field).
func Mapping(n *yaml.Node, path string) (map[string]*yaml.Node, error) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping", path)
	}
	return mappingMembers(n, path)
}

func mappingMembers(n *yaml.Node, path string) (map[string]*yaml.Node, error) {
	members := make(map[string]*yaml.Node)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
			return nil, at(k, path, "mapping keys must be strings")
		}
		if _, exists := members[k.Value]; exists {
			return nil, at(k, path+"."+k.Value, "duplicate field")
		}
		members[k.Value] = v
	}
	return members, nil
}

func blockStyle(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		n.Style &^= yaml.FlowStyle
	}
	for _, child := range n.Content {
		blockStyle(child)
	}
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Content = nil
	for _, child := range n.Content {
		c.Content = append(c.Content, cloneNode(child))
	}
	return &c
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }
func scalar(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}
func at(n *yaml.Node, path, format string, args ...any) error {
	where := path
	if n.Line > 0 {
		where += fmt.Sprintf(" (line %d, column %d)", n.Line, n.Column)
	}
	return fmt.Errorf("%s: %s", where, fmt.Sprintf(format, args...))
}
