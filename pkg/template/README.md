# Overview
This package contains the same code as Go's `text/template` and `html/template` packages, patched to avoid disabling dead code elimination.

## Code changes
The only functional change is that you can't call methods.
The types like `FuncMap` or `HTML` are aliases of standard library types, so there is no conflict when using one with the other.

## Organization
The [text](./text) directory contains the code from `text/template`, the [html](./html) directory contains the code from `html/template`.

## Code Generation
The code in this directory can be re-generated using `invoke -e pkg-template.generate` from the root of the repository.

## Go version
The code is from the Go version pinned in the [.go-version](/.go-version) file.
