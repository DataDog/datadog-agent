// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"sort"
	"strings"
)

// The CEL type tree describes the SECL fields as a tree of object types, so
// that a CEL environment can declare one variable holding a type with a
// `process` member, which in turn has a `file` member, rather than declaring
// every dotted name as its own variable.
//
// The tree is built from the SECL field *names*, not from the Go structs behind
// them. Those two do not agree: `connect.addr` draws its members from a nested
// `IPPortContext` and from flat Go fields tagged `addr.hostname` and
// `addr.family`, so no Go type can describe it. A trie over the names is
// indifferent to where a segment came from, which makes every such case
// uniform.

// CEL value shapes. They mirror StructField.GetEvaluatorType, minus the
// list-ness an enclosing iterator contributes: the tree carries that on the node
// instead, so a field of an iterated element has the type it has for a single
// element.
const (
	CELKindString = "string"
	CELKindInt    = "int"
	CELKindBool   = "bool"
	CELKindCIDR   = "cidr"
)

// CELMember is one member of a synthesized CEL object type.
type CELMember struct {
	// Name is the SECL path segment naming this member.
	Name string
	// Kind is the value shape of a leaf member, empty for a member that is
	// itself an object.
	Kind string
	// Shape names the object type of a non-leaf member, empty for a leaf.
	Shape string
	// List reports whether the member holds several values, either because it
	// is iterated or because the leaf itself holds a slice.
	List bool
}

// CELShape is a synthesized CEL object type.
type CELShape struct {
	// Name is the CEL type name, e.g. "secl.ProcessFile".
	Name string
	// Members are sorted by name.
	Members []CELMember
	// Path is the SECL path this type describes. There is one type per path
	// rather than one per member set, which is what lets a chain of selects be
	// resolved to the one field it reads.
	Path string
}

// CELElementRead names the function that reads an iterated field as a list of
// one element type.
//
// There is one per iterated node rather than one for all of them, because the
// element type has to survive into the expression: a member select on an
// iteration variable is type-checked against it, and it is what tells the
// optimization pass which field an index belongs to.
type CELElementRead struct {
	// TypeName is the element type, e.g. "secl.ProcessAncestors".
	TypeName string
	// FuncName is the CEL function that returns a list of it.
	FuncName string
}

// CELTypeTree is the generated view of the SECL field namespace.
type CELTypeTree struct {
	// Shapes are sorted by name, and include the root.
	Shapes []CELShape
	// Root is the name of the type the whole namespace hangs under, whose
	// members are the top level segments. It is the one type a CEL environment
	// declares a variable for.
	Root string
	// ElementReads are the iterated element types, sorted by type name.
	ElementReads []CELElementRead
	// UsesCIDR reports whether any field is IP or CIDR valued, which decides
	// whether the generated file needs the network extension library.
	UsesCIDR bool
}

// celTypePrefix namespaces the generated CEL type names.
const celTypePrefix = "secl."

// celReadPrefix namespaces the generated read functions, which is how a field
// reaches the interpreter: `secl.readProcessAncestors(evt, 12)`.
const celReadPrefix = celTypePrefix + "read"

// celRootType is the type of the whole field namespace. The top level segments
// are its members rather than declarations of their own, so that every field
// select — including the first — is bound to the field it denotes when a rule is
// planned, and so that a macro or a constant cannot be named after a segment.
const celRootType = celTypePrefix + "Root"

type celNode struct {
	children map[string]*celNode

	isLeaf bool
	kind   string
	list   bool

	shape string
}

// BuildCELTypeTree derives the CEL type tree from the SECL fields of a module.
//
// It returns an error when the field set cannot be expressed as a tree, so that
// a future field or tag that breaks the invariant fails the build instead of
// silently producing a type nobody can select through.
func BuildCELTypeTree(module *Module) (*CELTypeTree, error) {
	root := &celNode{children: map[string]*celNode{}}

	names := make([]string, 0, len(module.Fields))
	for name := range module.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		field := module.Fields[name]

		// `length` and `root_domain` are the only leaves that are also a prefix
		// of another leaf: `x.length` exists alongside `x`, and `x` cannot be
		// both a string and an object with a `length` member. They are rewritten
		// to size() and secl.rootDomain() at translation time instead.
		if field.GettersOnly || field.IsLength || field.IsRootDomain {
			continue
		}

		segments := strings.Split(name, ".")
		node := root
		for i, segment := range segments {
			if segment == "" {
				return nil, fmt.Errorf("field %q has an empty path segment", name)
			}
			child, ok := node.children[segment]
			if !ok {
				child = &celNode{children: map[string]*celNode{}}
				node.children[segment] = child
			}
			if child.isLeaf && i != len(segments)-1 {
				return nil, fmt.Errorf("field %q needs %q to be an object, but it is a value",
					name, strings.Join(segments[:i+1], "."))
			}
			node = child
		}

		if len(node.children) != 0 {
			return nil, fmt.Errorf("field %q is a value but is also the prefix of another field", name)
		}
		node.isLeaf = true
		node.kind = celKindOf(field)
		// An iterated field is a list because of the iterator, which the
		// enclosing node already carries; only a genuine slice makes the leaf
		// itself a list.
		node.list = field.IsArray && !field.IsLength
	}

	// Iterators are keyed by their SECL path, which is the node whose elements
	// are iterated.
	iterators := make([]string, 0, len(module.Iterators))
	for path := range module.Iterators {
		iterators = append(iterators, path)
	}
	sort.Strings(iterators)

	for _, path := range iterators {
		node := root
		for _, segment := range strings.Split(path, ".") {
			child, ok := node.children[segment]
			if !ok {
				node = nil
				break
			}
			node = child
		}
		// An iterator with no exposed field under it has no node in the tree.
		if node == nil || node == root {
			continue
		}
		if node.isLeaf {
			return nil, fmt.Errorf("iterator %q is also a value field", path)
		}
		node.list = true
	}

	tree := &CELTypeTree{Root: celRootType}
	if err := nameShapes(root, tree); err != nil {
		return nil, err
	}

	elements := map[string]bool{}
	for _, shape := range tree.Shapes {
		for _, member := range shape.Members {
			if member.Kind == CELKindCIDR {
				tree.UsesCIDR = true
			}
			// An iterated member is a list of objects, and its element type is what
			// a read of that member returns.
			if member.List && member.Shape != "" {
				elements[member.Shape] = true
			}
		}
	}

	for _, name := range sortedNames(elements) {
		tree.ElementReads = append(tree.ElementReads, CELElementRead{
			TypeName: name,
			FuncName: celReadPrefix + strings.TrimPrefix(name, celTypePrefix),
		})
	}

	return tree, nil
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// celKindOf maps a field's return type onto a CEL value shape. It deliberately
// mirrors StructField.GetEvaluatorType so the two cannot disagree about what a
// field holds.
func celKindOf(field *StructField) string {
	switch field.ReturnType {
	case "int":
		return CELKindInt
	case "bool":
		return CELKindBool
	case "net.IPNet":
		return CELKindCIDR
	default:
		return CELKindString
	}
}

// nameShapes assigns a CEL type name to every internal node, one per path.
//
// Sharing a name between nodes exposing the same members would declare fewer
// types — 84 rather than 276 — but a shared type cannot say which field a member
// denotes. One type per path can: it is what lets the optimization pass resolve a
// chain of selects to one field, from the type of the expression the chain hangs
// off, and rewrite it into a read of that field.
//
// The cost is that two paths exposing the same members are no longer the same
// type, so nothing can be written polymorphically over them. SECL has no such
// construct today; macros would be where it started to matter.
func nameShapes(root *celNode, tree *CELTypeTree) error {
	taken := map[string]string{} // type name -> the path that claimed it

	var walk func(node *celNode, path string) error
	walk = func(node *celNode, path string) error {
		if node.isLeaf {
			return nil
		}

		// The root holds the whole namespace, so it is named rather than
		// camel cased from its empty path.
		name := celRootType
		if node != root {
			name = celTypePrefix + camelPath(strings.Split(path, "."))
		}
		// Paths are unique, but two of them can camel case to one name:
		// `x.file_metadata` and `x.file.metadata` both give FileMetadata. The root
		// is named first, so a top level `root` field collides with it here rather
		// than silently taking its place.
		if claimed, ok := taken[name]; ok {
			return fmt.Errorf("paths %s and %s both name the type %q", pathLabel(claimed), pathLabel(path), name)
		}
		taken[name] = path
		node.shape = name

		for _, segment := range sortedKeys(node.children) {
			if err := walk(node.children[segment], join(path, segment)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return err
	}

	// Second pass, now that every node knows its name, to fill in the members
	// that point at other shapes.
	var emit func(node *celNode, path string)
	emit = func(node *celNode, path string) {
		if node.isLeaf {
			return
		}
		tree.Shapes = append(tree.Shapes, CELShape{
			Name:    node.shape,
			Members: membersOf(node),
			Path:    path,
		})
		for _, segment := range sortedKeys(node.children) {
			emit(node.children[segment], join(path, segment))
		}
	}
	emit(root, "")

	sort.Slice(tree.Shapes, func(i, j int) bool { return tree.Shapes[i].Name < tree.Shapes[j].Name })

	return nil
}

// pathLabel names a path in an error message. The root has no path.
func pathLabel(path string) string {
	if path == "" {
		return "the root"
	}
	return fmt.Sprintf("%q", path)
}

// camelPath joins SECL path segments into a Go style type name.
func camelPath(segments []string) string {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString(camel(segment))
	}
	return b.String()
}

func membersOf(node *celNode) []CELMember {
	members := make([]CELMember, 0, len(node.children))
	for _, segment := range sortedKeys(node.children) {
		child := node.children[segment]
		member := CELMember{Name: segment, List: child.list}
		if child.isLeaf {
			member.Kind = child.kind
		} else {
			member.Shape = child.shape
		}
		members = append(members, member)
	}
	return members
}

func sortedKeys(m map[string]*celNode) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func join(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}

// camel turns a SECL segment into a Go style type name, e.g. "file_metadata"
// into "FileMetadata".
func camel(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '_' {
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		upper = false
		b.WriteRune(r)
	}
	return b.String()
}
