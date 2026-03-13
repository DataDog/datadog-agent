// Vendored from github.com/wasilibs/go-re2 v1.10.0 internal/cre2/cre2.go
// See ../go-re2/LICENSE for the original MIT license.

//go:build re2_cgo

package cre2

/*
#include <stdbool.h>
#include <stdint.h>

void* malloc(size_t size);
void free(void* ptr);

void* cre2_new(void* pattern, int pattern_len, void* opts);
void cre2_delete(void* re);
int cre2_error_code(void* re);
void* cre2_error_arg(void* re);
int cre2_match(void* re, void* text, int text_len, int startpos, int endpos, int anchor, void* match_arr, int nmatch);
int cre2_num_capturing_groups(void* re);
void* cre2_named_groups_iter_new(void* re);
bool cre2_named_groups_iter_next(void* iter, void** name, int* index);
void cre2_named_groups_iter_delete(void* iter);

void* cre2_opt_new();
void cre2_opt_delete(void* opts);
void cre2_opt_set_log_errors(void* opt, int flag);
void cre2_opt_set_longest_match(void* opt, int flag);
void cre2_opt_set_posix_syntax(void* opt, int flag);
void cre2_opt_set_case_sensitive(void* opt, int flag);
void cre2_opt_set_latin1_encoding(void* opt);
void cre2_opt_set_max_mem(void* opt, int64_t size);

*/
import "C"

import (
	"unsafe"
)

// New compiles a new RE2 regexp from pattern with the given options.
func New(patternPtr unsafe.Pointer, patternLen int, opts unsafe.Pointer) unsafe.Pointer {
	return C.cre2_new(patternPtr, C.int(patternLen), opts)
}

// Delete frees a compiled RE2 regexp.
func Delete(ptr unsafe.Pointer) {
	C.cre2_delete(ptr)
}

// ErrorCode returns the error code for a compiled regexp (0 = no error).
func ErrorCode(rePtr unsafe.Pointer) int {
	return int(C.cre2_error_code(rePtr))
}

// ErrorArg returns a pointer to the error argument string.
func ErrorArg(rePtr unsafe.Pointer) unsafe.Pointer {
	return C.cre2_error_arg(rePtr)
}

// Match tests whether text matches the compiled regexp.
func Match(rePtr unsafe.Pointer, textPtr unsafe.Pointer, textLen int, startPos int, endPos int, anchor int, matchArr unsafe.Pointer, nMatch int) bool {
	return C.cre2_match(rePtr, textPtr, C.int(textLen), C.int(startPos), C.int(endPos), C.int(anchor), matchArr, C.int(nMatch)) > 0
}

// NamedGroupsIterNew creates a new iterator over named capturing groups.
func NamedGroupsIterNew(rePtr unsafe.Pointer) unsafe.Pointer {
	return C.cre2_named_groups_iter_new(rePtr)
}

// NamedGroupsIterNext advances the iterator and returns the next named group.
func NamedGroupsIterNext(iterPtr unsafe.Pointer, namePtr *unsafe.Pointer, indexPtr *int) bool {
	cIndex := C.int(0)
	res := C.cre2_named_groups_iter_next(iterPtr, namePtr, &cIndex)
	*indexPtr = int(cIndex)
	return bool(res)
}

// NamedGroupsIterDelete frees a named groups iterator.
func NamedGroupsIterDelete(iterPtr unsafe.Pointer) {
	C.cre2_named_groups_iter_delete(iterPtr)
}

// NumCapturingGroups returns the number of capturing groups in the regexp.
func NumCapturingGroups(rePtr unsafe.Pointer) int {
	return int(C.cre2_num_capturing_groups(rePtr))
}

// NewOpt creates a new RE2 options object.
func NewOpt() unsafe.Pointer {
	return C.cre2_opt_new()
}

// DeleteOpt frees an RE2 options object.
func DeleteOpt(opt unsafe.Pointer) {
	C.cre2_opt_delete(opt)
}

// OptSetLogErrors enables or disables error logging in the options.
func OptSetLogErrors(opt unsafe.Pointer, flag bool) {
	C.cre2_opt_set_log_errors(opt, cFlag(flag))
}

// OptSetLongestMatch enables or disables longest-match semantics.
func OptSetLongestMatch(opt unsafe.Pointer, flag bool) {
	C.cre2_opt_set_longest_match(opt, cFlag(flag))
}

// OptSetPosixSyntax enables or disables POSIX syntax.
func OptSetPosixSyntax(opt unsafe.Pointer, flag bool) {
	C.cre2_opt_set_posix_syntax(opt, cFlag(flag))
}

// OptSetCaseSensitive enables or disables case-sensitive matching.
func OptSetCaseSensitive(opt unsafe.Pointer, flag bool) {
	C.cre2_opt_set_case_sensitive(opt, cFlag(flag))
}

// OptSetLatin1Encoding sets the encoding to Latin-1.
func OptSetLatin1Encoding(opt unsafe.Pointer) {
	C.cre2_opt_set_latin1_encoding(opt)
}

// OptSetMaxMem sets the maximum memory for the regexp.
func OptSetMaxMem(opt unsafe.Pointer, size int) {
	C.cre2_opt_set_max_mem(opt, C.int64_t(size))
}

// Malloc allocates memory via C malloc.
func Malloc(size int) unsafe.Pointer {
	return C.malloc(C.size_t(size))
}

// Free frees memory allocated by C malloc.
func Free(ptr unsafe.Pointer) {
	C.free(ptr)
}

// CopyCString copies a C string into a Go string.
func CopyCString(sPtr unsafe.Pointer) string {
	return C.GoString((*C.char)(sPtr))
}

func cFlag(flag bool) C.int {
	if flag {
		return C.int(1)
	}
	return C.int(0)
}
