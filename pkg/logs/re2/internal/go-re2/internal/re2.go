// Vendored from github.com/wasilibs/go-re2 v1.10.0 internal/re2.go
// See ../LICENSE for the original MIT license.

//go:build re2_cgo

package internal

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// Use same max size as regexp package.
// https://github.com/golang/go/blob/master/src/regexp/syntax/parse.go#L95
const maxSize = 128 << 20

type Regexp struct {
	ptr wasmPtr

	opts CompileOptions

	expr string

	numMatches     int
	groupNames     []string
	groupNamesOnce sync.Once

	abi *libre2ABI

	released uint32
}

type CompileOptions struct {
	Posix           bool
	Longest         bool
	CaseInsensitive bool
	Latin1          bool
}

func Compile(expr string, opts CompileOptions) (*Regexp, error) {
	abi := newABI()
	alloc := abi.startOperation(len(expr) + 2)
	defer abi.endOperation(alloc)

	cs := alloc.newCString(expr)

	rePtr := newRE(abi, cs, opts)
	errCode, errArg := reError(abi, rePtr)
	switch errCode {
	case 0:
	// No error.
	case 1:
		return nil, fmt.Errorf("error parsing regexp: unexpected error: %#q", errArg)
	case 2:
		return nil, fmt.Errorf("error parsing regexp: invalid escape sequence: %#q", errArg)
	case 3:
		return nil, fmt.Errorf("error parsing regexp: bad character class: %#q", errArg)
	case 4:
		return nil, fmt.Errorf("error parsing regexp: invalid character class range: %#q", errArg)
	case 5:
		return nil, fmt.Errorf("error parsing regexp: missing closing ]: %#q", errArg)
	case 6:
		return nil, fmt.Errorf("error parsing regexp: missing closing ): %#q", errArg)
	case 7:
		return nil, fmt.Errorf("error parsing regexp: unexpected ): %#q", errArg)
	case 8:
		return nil, fmt.Errorf("error parsing regexp: trailing backslash at end of expression: %#q", errArg)
	case 9:
		return nil, fmt.Errorf("error parsing regexp: missing argument to repetition operator: %#q", errArg)
	case 10:
		return nil, fmt.Errorf("error parsing regexp: bad repitition argument: %#q", errArg)
	case 11:
		return nil, fmt.Errorf("error parsing regexp: invalid nested repetition operator: %#q", errArg)
	case 12:
		return nil, fmt.Errorf("error parsing regexp: bad perl operator: %#q", errArg)
	case 13:
		return nil, fmt.Errorf("error parsing regexp: invalid UTF-8 in regexp: %#q", errArg)
	case 14:
		return nil, fmt.Errorf("error parsing regexp: bad named capture group: %#q", errArg)
	case 15:
		return nil, fmt.Errorf("error parsing regexp: expression too large")
	}

	// Does not include whole expression match, e.g. $0
	numGroups := numCapturingGroups(abi, rePtr)

	re := &Regexp{
		ptr:        rePtr,
		opts:       opts,
		expr:       expr,
		numMatches: numGroups + 1,
		abi:        abi,
	}

	// Use func(interface{}) form for nottinygc compatibility.
	runtime.SetFinalizer(re, func(obj interface{}) {
		obj.(*Regexp).release()
	})

	return re, nil
}

// Expand appends template to dst and returns the result; during the
// append, Expand replaces variables in the template with corresponding
// matches drawn from src. The match slice should have been returned by
// FindSubmatchIndex.
func (re *Regexp) Expand(dst []byte, template []byte, src []byte, match []int) []byte {
	return re.expand(dst, string(template), src, "", match)
}

func (re *Regexp) expand(dst []byte, template string, bsrc []byte, src string, match []int) []byte {
	for len(template) > 0 {
		before, after, ok := strings.Cut(template, "$")
		if !ok {
			break
		}
		dst = append(dst, before...)
		template = after
		if template != "" && template[0] == '$' {
			// Treat $$ as $.
			dst = append(dst, '$')
			template = template[1:]
			continue
		}
		name, num, rest, ok := extract(template)
		if !ok {
			// Malformed; treat $ as raw text.
			dst = append(dst, '$')
			continue
		}
		template = rest
		if num >= 0 {
			if 2*num+1 < len(match) && match[2*num] >= 0 {
				if bsrc != nil {
					dst = append(dst, bsrc[match[2*num]:match[2*num+1]]...)
				} else {
					dst = append(dst, src[match[2*num]:match[2*num+1]]...)
				}
			}
		} else {
			for i, namei := range re.SubexpNames() {
				if name == namei && 2*i+1 < len(match) && match[2*i] >= 0 {
					if bsrc != nil {
						dst = append(dst, bsrc[match[2*i]:match[2*i+1]]...)
					} else {
						dst = append(dst, src[match[2*i]:match[2*i+1]]...)
					}
					break
				}
			}
		}
	}

	runtime.KeepAlive(re) // don't allow finalizer to run during method

	dst = append(dst, template...)
	return dst
}

// FindAllIndex is the 'All' version of FindIndex; it returns a slice of all
// successive matches of the expression, as defined by the 'All' description
// in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllIndex(b []byte, n int) [][]int {
	alloc := re.abi.startOperation(len(b) + 16)
	defer re.abi.endOperation(alloc)

	cs := alloc.newCStringFromBytes(b)

	var matches [][]int

	re.findAll(&alloc, b, "", cs, n, func(match []int) {
		matches = append(matches, append([]int(nil), match...))
	})

	res := matches
	runtime.KeepAlive(b)
	return res
}

func (re *Regexp) findAll(alloc *allocation, bsrc []byte, src string, cs cString, n int, deliver func(match []int)) {
	// Fast path: skip match-array malloc/free when there is no match.
	if !match(re, cs, nilWasmPtr, 0) {
		runtime.KeepAlive(re)
		return
	}

	var dstCap [2]int

	if n < 0 {
		n = cs.length + 1
	}

	matchArr := alloc.newCStringArray(1)
	defer matchArr.free()

	count := 0
	prevMatchEnd := -1
	pos := 0
	for pos < cs.length+1 {
		if !matchFrom(re, cs, pos, matchArr.ptr, 1) {
			break
		}

		match := readMatch(alloc, cs, matchArr.ptr, dstCap[:0])
		accept := true
		// Check if it's an empty match following a match, which we ignore.
		if match[0] == match[1] && match[0] == prevMatchEnd {
			// We don't allow an empty match right
			// after a previous match, so ignore it.
			accept = false
		}

		pos = nextPos(bsrc, src, pos, match[1])

		if accept {
			deliver(match)
			count++
		}
		prevMatchEnd = match[1]

		if count == n {
			break
		}
	}

	runtime.KeepAlive(matchArr)

	runtime.KeepAlive(re) // don't allow finalizer to run during method
}

// FindAllSubmatchIndex is the 'All' version of FindSubmatchIndex; it returns
// a slice of all successive matches of the expression, as defined by the
// 'All' description in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllSubmatchIndex(b []byte, n int) [][]int {
	alloc := re.abi.startOperation(len(b) + 8*re.numMatches + 8)
	defer re.abi.endOperation(alloc)

	cs := alloc.newCStringFromBytes(b)

	var matches [][]int

	re.findAllSubmatch(&alloc, b, "", cs, re.numMatches, n, func(match []int) {
		matches = append(matches, match)
	})

	res := matches
	runtime.KeepAlive(b)
	return res
}

func (re *Regexp) findAllSubmatch(alloc *allocation, bsrc []byte, src string, cs cString, nmatch, n int, deliver func(match []int)) {
	// Fast path: skip match-array malloc/free when there is no match.
	if !match(re, cs, nilWasmPtr, 0) {
		runtime.KeepAlive(re)
		return
	}

	if n < 0 {
		n = cs.length + 1
	}

	matchArr := alloc.newCStringArray(nmatch)
	defer matchArr.free()

	count := 0
	prevMatchEnd := -1
	pos := 0
	for pos < cs.length+1 {
		if !matchFrom(re, cs, pos, matchArr.ptr, uint32(nmatch)) {
			break
		}

		var matches []int
		accept := true
		readMatches(alloc, cs, matchArr.ptr, nmatch, func(match []int) bool {
			if len(matches) == 0 {
				// First match, check if it's an empty match following a match, which we ignore.
				if match[0] == match[1] && match[0] == prevMatchEnd {
					accept = false
				}

				pos = nextPos(bsrc, src, pos, match[1])
				prevMatchEnd = match[1]
			}
			if accept {
				matches = append(matches, match...)
				return true
			} else {
				return false
			}
		})
		if accept {
			deliver(matches)
		}
		count++

		if count == n {
			break
		}
	}

	runtime.KeepAlive(matchArr)

	runtime.KeepAlive(re) // don't allow finalizer to run during method
}

// NumSubexp returns the number of parenthesized subexpressions in this Regexp.
func (re *Regexp) NumSubexp() int {
	return re.numMatches - 1
}

// SubexpNames returns the names of the parenthesized subexpressions
// in this Regexp. The name for the first sub-expression is names[1],
// so that if m is a match slice, the name for m[i] is SubexpNames()[i].
// Since the Regexp as a whole cannot be named, names[0] is always
// the empty string. The slice should not be modified.
func (re *Regexp) SubexpNames() []string {
	re.groupNamesOnce.Do(func() {
		re.groupNames = subexpNames(re.abi, re.ptr, re.numMatches)
	})
	return re.groupNames
}

// String returns the source text used to compile the regular expression.
func (re *Regexp) String() string {
	return re.expr
}

func (re *Regexp) release() {
	if !atomic.CompareAndSwapUint32(&re.released, 0, 1) {
		return
	}
	release(re)
}

func subexpNames(abi *libre2ABI, rePtr wasmPtr, numMatches int) []string {
	res := make([]string, numMatches)

	iter := namedGroupsIter(abi, rePtr)
	defer namedGroupsIterDelete(abi, iter)

	for {
		name, index, ok := namedGroupsIterNext(abi, iter)
		if !ok {
			break
		}
		res[index] = name
	}

	return res
}

// extract returns the name from a leading "name" or "{name}" in str.
// (The $ has already been removed by the caller.)
// If it is a number, extract returns num set to that number; otherwise num = -1.
// Copied as is from
// https://github.com/golang/go/blob/0fd7be7ee5f36215b5d6b8f23f35d60bf749805a/src/regexp/regexp.go#L981
func extract(str string) (name string, num int, rest string, ok bool) {
	if str == "" {
		return
	}
	brace := false
	if str[0] == '{' {
		brace = true
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		rune, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(rune) && !unicode.IsDigit(rune) && rune != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		// empty name is not okay
		return
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			// missing closing brace
			return
		}
		i++
	}

	// Parse number.
	num = 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || '9' < name[i] || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[i]) - '0'
	}
	// Disallow leading zeros.
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}

	rest = str[i:]
	ok = true
	return
}

func nextPos(bsrc []byte, src string, pos int, matchEnd int) int {
	// Advance past the match; always advance at least one character.
	var width int
	if bsrc != nil {
		_, width = utf8.DecodeRune(bsrc[pos:])
	} else {
		_, width = utf8.DecodeRuneInString(src[pos:])
	}

	if pos+width > matchEnd {
		return pos + width
	} else if pos+1 > matchEnd {
		// This clause is only needed at the end of the input
		// string. In that case, DecodeRuneInString returns width=0.
		return pos + 1
	} else {
		return matchEnd
	}
}
