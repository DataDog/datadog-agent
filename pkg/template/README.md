# Overview
This package contains the same code as Go's `text/template` and `html/template` packages, patched to avoid disabling dead code elimination.

## Code changes
The only functional change is that you can't call methods.

## Organization
The [text](./text) directory contains the code from `text/template`, the [html](./html) directory contains the code from `html/template`.

## Go version
The code is from the Go version pinned in the [.go-version](/.go-version) file.
