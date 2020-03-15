package jsonpatch

import (
	"encoding/json"
	"fmt"
)

// An Operation represents a JSON Patch Operation for:
// https://tools.ietf.org/html/rfc6902
type Operation interface {
	sealed()
}

// A Patch document is a JSON [RFC4627] document that represents an
// array of objects.  Each object represents a single operation to be
// applied to the target JSON document.
type Patch []Operation

func (patch Patch) String() string {
	bs, _ := json.MarshalIndent([]Operation(patch), "", "  ")
	return string(bs)
}

// UnmarshalJSON unmarshals JSON into a patch
func (patch *Patch) UnmarshalJSON(data []byte) error {
	var unionops []struct {
		Op    string      `json:"op"`
		Path  string      `json:"path"`
		Value interface{} `json:"value"`
		From  string      `json:"from"`
	}
	err := json.Unmarshal(data, &unionops)
	if err != nil {
		return err
	}
	*patch = Patch(make([]Operation, len(unionops)))
	for i := range *patch {
		unionop := unionops[i]
		switch unionop.Op {
		case "add":
			(*patch)[i] = Add(unionop.Path, unionop.Value)
		case "remove":
			(*patch)[i] = Remove(unionop.Path)
		case "replace":
			(*patch)[i] = Replace(unionop.Path, unionop.Value)
		case "move":
			(*patch)[i] = Move(unionop.Path, unionop.From)
		case "copy":
			(*patch)[i] = Copy(unionop.Path, unionop.From)
		case "test":
			(*patch)[i] = Test(unionop.Path, unionop.Value)
		default:
			return fmt.Errorf("invalid operation: %s", unionop.Op)
		}
	}
	return nil
}

// New creates a new Patch
func New(operations ...Operation) Patch {
	return operations
}

type addOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
type removeOperation struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}
type replaceOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
type moveOperation struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	From string `json:"from"`
}
type copyOperation struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	From string `json:"from"`
}
type testOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func (op addOperation) sealed()     {}
func (op removeOperation) sealed()  {}
func (op replaceOperation) sealed() {}
func (op moveOperation) sealed()    {}
func (op copyOperation) sealed()    {}
func (op testOperation) sealed()    {}

// Add creates an add operation
//
// The "add" operation performs one of the following functions,
// depending upon what the target location references:
//
// o  If the target location specifies an array index, a new value is
//    inserted into the array at the specified index.
//
// o  If the target location specifies an object member that does not
//    already exist, a new member is added to the object.
//
// o  If the target location specifies an object member that does exist,
//    that member's value is replaced.
//
// The operation object MUST contain a "value" member whose content
// specifies the value to be added.
//
// For example:
//
// { "op": "add", "path": "/a/b/c", "value": [ "foo", "bar" ] }
//
// When the operation is applied, the target location MUST reference one
// of:
//
// o  The root of the target document - whereupon the specified value
//    becomes the entire content of the target document.
//
// o  A member to add to an existing object - whereupon the supplied
//    value is added to that object at the indicated location.  If the
//    member already exists, it is replaced by the specified value.
//
// o  An element to add to an existing array - whereupon the supplied
//    value is added to the array at the indicated location.  Any
//    elements at or above the specified index are shifted one position
//    to the right.  The specified index MUST NOT be greater than the
//    number of elements in the array.  If the "-" character is used to
//    index the end of the array (see [RFC6901]), this has the effect of
//    appending the value to the array.
//
// Because this operation is designed to add to existing objects and
// arrays, its target location will often not exist.  Although the
// pointer's error handling algorithm will thus be invoked, this
// specification defines the error handling behavior for "add" pointers
// to ignore that error and add the value as specified.
//
// However, the object itself or an array containing it does need to
// exist, and it remains an error for that not to be the case.  For
// example, an "add" with a target location of "/a/b" starting with this
// document:
//
// { "a": { "foo": 1 } }
//
// is not an error, because "a" exists, and "b" will be added to its
// value.  It is an error in this document:
//
// { "q": { "bar": 2 } }
//
// because "a" does not exist.
func Add(path string, value interface{}) Operation {
	return addOperation{
		Op:    "add",
		Path:  path,
		Value: value,
	}
}

// Remove creates a new remove operation
//
// The "remove" operation removes the value at the target location.
//
// The target location MUST exist for the operation to be successful.
//
// For example:
//
//     { "op": "remove", "path": "/a/b/c" }
//
// If removing an element from an array, any elements above the
// specified index are shifted one position to the left.
func Remove(path string) Operation {
	return removeOperation{
		Op:   "remove",
		Path: path,
	}
}

// Replace creates a new replace operation
//
// The "replace" operation replaces the value at the target location
// with a new value.  The operation object MUST contain a "value" member
// whose content specifies the replacement value.
//
// The target location MUST exist for the operation to be successful.
//
// For example:
//
// { "op": "replace", "path": "/a/b/c", "value": 42 }
//
// This operation is functionally identical to a "remove" operation for
// a value, followed immediately by an "add" operation at the same
// location with the replacement value.
func Replace(path string, value interface{}) Operation {
	return replaceOperation{
		Op:    "replace",
		Path:  path,
		Value: value,
	}
}

// Move creates a new move operation
//
// The "move" operation removes the value at a specified location and
// adds it to the target location.
//
// The operation object MUST contain a "from" member, which is a string
// containing a JSON Pointer value that references the location in the
// target document to move the value from.
//
// The "from" location MUST exist for the operation to be successful.
//
// For example:
//
// { "op": "move", "from": "/a/b/c", "path": "/a/b/d" }
//
// This operation is functionally identical to a "remove" operation on
// the "from" location, followed immediately by an "add" operation at
// the target location with the value that was just removed.
//
// The "from" location MUST NOT be a proper prefix of the "path"
// location; i.e., a location cannot be moved into one of its children.
func Move(path, from string) Operation {
	return moveOperation{
		Op:   "move",
		Path: path,
		From: from,
	}
}

// Copy creates a new copy operation
//
// The "copy" operation copies the value at a specified location to the
// target location.
//
// The operation object MUST contain a "from" member, which is a string
// containing a JSON Pointer value that references the location in the
// target document to copy the value from.
//
// The "from" location MUST exist for the operation to be successful.
//
// For example:
//
// { "op": "copy", "from": "/a/b/c", "path": "/a/b/e" }
//
// This operation is functionally identical to an "add" operation at the
// target location using the value specified in the "from" member.
func Copy(path, from string) Operation {
	return copyOperation{
		Op:   "copy",
		Path: path,
		From: from,
	}
}

// Test creates a new test operation
//
// The "test" operation tests that a value at the target location is
// equal to a specified value.
//
// The operation object MUST contain a "value" member that conveys the
// value to be compared to the target location's value.
//
// The target location MUST be equal to the "value" value for the
// operation to be considered successful.
//
// Here, "equal" means that the value at the target location and the
// value conveyed by "value" are of the same JSON type, and that they
// are considered equal by the following rules for that type:
//
// o  strings: are considered equal if they contain the same number of
//    Unicode characters and their code points are byte-by-byte equal.
//
// o  numbers: are considered equal if their values are numerically
//    equal.
//
// o  arrays: are considered equal if they contain the same number of
//    values, and if each value can be considered equal to the value at
//    the corresponding position in the other array, using this list of
//    type-specific rules.
//
// o  objects: are considered equal if they contain the same number of
//    members, and if each member can be considered equal to a member in
//    the other object, by comparing their keys (as strings) and their
//    values (using this list of type-specific rules).
//
// o  literals (false, true, and null): are considered equal if they are
//    the same.
//
// Note that the comparison that is done is a logical comparison; e.g.,
// whitespace between the member values of an array is not significant.
//
// Also, note that ordering of the serialization of object members is
// not significant.
//
// For example:
//
// { "op": "test", "path": "/a/b/c", "value": "foo" }
func Test(path string, value interface{}) Operation {
	return testOperation{
		Op:    "test",
		Path:  path,
		Value: value,
	}
}
