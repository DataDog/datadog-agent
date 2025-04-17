package expansion

/*
NOTE: The MappingFuncFor and Expand method in this 3rd party package have
been modified. Changed functionality involves also returning a boolean
indicating whether all the variables in the input were found in the provided context.
*/

import (
	"bytes"
)

const (
	operator        = '$'
	referenceOpener = '('
	referenceCloser = ')'
)

// syntaxWrap returns the input string wrapped by the expansion syntax.
func syntaxWrap(input string) string {
	return string(operator) + string(referenceOpener) + input + string(referenceCloser)
}

// MappingFuncFor returns a mapping function for use with Expand that
// implements the expansion semantics defined in the expansion spec; it
// returns the input string wrapped in the expansion syntax if no mapping
// for the input is found.
//
// Additionally, it returns a boolean indicating whether the variable was
// found in the context.
func MappingFuncFor(context ...map[string]string) func(string) (string, bool) {
	return func(input string) (string, bool) {
		for _, vars := range context {
			val, ok := vars[input]
			if ok {
				return val, true
			}
		}

		return syntaxWrap(input), false
	}
}

// Expand replaces variable references in the input string according to
// the expansion spec using the given mapping function to resolve the
// values of variables.
//
// Additionally, it returns the status of whether all nested variables
// have a defined mapping value in the environment.
func Expand(input string, mapping func(string) (string, bool)) (string, bool) {
	var buf bytes.Buffer
	checkpoint := 0
	allMappingsFound := true
	for cursor := 0; cursor < len(input); cursor++ {
		if input[cursor] == operator && cursor+1 < len(input) {
			// Copy the portion of the input string since the last
			// checkpoint into the buffer
			buf.WriteString(input[checkpoint:cursor])

			// Attempt to read the variable name as defined by the
			// syntax from the input string
			read, isVar, advance := tryReadVariableName(input[cursor+1:])

			if isVar {
				// We were able to read a variable name correctly;
				// apply the mapping to the variable name and copy the
				// bytes into the buffer
				mappedValue, found := mapping(read)

				// Record that the read variable is not mapped in the environment
				if !found {
					allMappingsFound = false
				}

				buf.WriteString(mappedValue)
			} else {
				// Not a variable name; copy the read bytes into the buffer
				buf.WriteString(read)
			}

			// Advance the cursor in the input string to account for
			// bytes consumed to read the variable name expression
			cursor += advance

			// Advance the checkpoint in the input string
			checkpoint = cursor + 1
		}
	}

	// Return the buffer and any remaining unwritten bytes in the
	// input string. Also return whether any nested variables in
	// the input string were not found in the environment.
	return buf.String() + input[checkpoint:], allMappingsFound
}

// tryReadVariableName attempts to read a variable name from the input
// string and returns the content read from the input, whether that content
// represents a variable name to perform mapping on, and the number of bytes
// consumed in the input string.
//
// The input string is assumed not to contain the initial operator.
func tryReadVariableName(input string) (string, bool, int) {
	switch input[0] {
	case operator:
		// Escaped operator; return it.
		return input[0:1], false, 1
	case referenceOpener:
		// Scan to expression closer
		for i := 1; i < len(input); i++ {
			if input[i] == referenceCloser {
				return input[1:i], true, i + 1
			}
		}

		// Incomplete reference; return it.
		return string(operator) + string(referenceOpener), false, 1
	default:
		// Not the beginning of an expression, ie, an operator
		// that doesn't begin an expression.  Return the operator
		// and the first rune in the string.
		return (string(operator) + string(input[0])), false, 1
	}
}
