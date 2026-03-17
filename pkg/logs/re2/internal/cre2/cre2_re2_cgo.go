// Vendored from github.com/wasilibs/go-re2 v1.10.0 internal/cre2/cre2_re2_cgo.go
// See ../go-re2/LICENSE for the original MIT license.
// Modified: links pre-built libcre2.a (compiled by Bazel during Omnibus build)
// and libre2.so instead of compiling cre2.cpp from source via CGo.

//go:build re2_cgo

package cre2

/*
#cgo LDFLAGS: -lcre2 -lre2 -lstdc++ -lpthread
*/
import "C"
