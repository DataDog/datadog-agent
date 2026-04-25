// Vendored from github.com/wasilibs/go-re2 v1.10.0 internal/cre2/cre2_re2_cgo.go
// See ../go-re2/LICENSE for the original MIT license.
// Modified: links a fat static archive (libcre2.a) containing cre2 + RE2 +
// Abseil, built by Bazel during the Omnibus build. libstdc++ is linked
// statically so the agent has no runtime C++ shared-library dependency.

//go:build re2_cgo

package cre2

/*
#cgo LDFLAGS: -lcre2 -Wl,-Bstatic -lstdc++ -Wl,-Bdynamic -lm -lpthread
*/
import "C"
