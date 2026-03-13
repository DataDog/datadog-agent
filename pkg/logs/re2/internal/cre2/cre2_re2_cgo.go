// Vendored from github.com/wasilibs/go-re2 v1.10.0 internal/cre2/cre2_re2_cgo.go
// See ../go-re2/LICENSE for the original MIT license.
// Modified: uses -lre2 linker flag instead of pkg-config.

//go:build re2_cgo

package cre2

/*
#cgo LDFLAGS: -lre2 -lstdc++ -lpthread
#cgo CXXFLAGS: -std=c++17
*/
import "C"
