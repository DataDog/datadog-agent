# `github.com/go-logr/logr/funcr` patch

## What is this?

This is an implementation of the `github.com/go-logr/logr/funcr` package from `github.com/go-logr/logr@v1.2.2` which is compatible with v0.4.0. 

It is exposed as a module and declares its path to be `github.com/go-logr/logr/funcr`.

## What is the motivation behind this?

We need this because of a 'dependency hell' situation:

1. `k8s.io/component-base` depends on v0.4.0 of `github.com/go-logr/logr`
2. `go.opentelemetry.io/otel` depends on v1.2.2 of `github.com/go-logr/logr`.
3. Go considers v0.x and v1.x to be the same major version, pressumably for backwards compatibility with the era pre-modules.

This situation would be solved by bumping Kubernetes to v0.23.0 or above, see [relevant commit](https://github.com/kubernetes/kubernetes/commit/cb6a6537).
However, Kubernetes can't be upgraded above v0.21.x, because of another 'dependency hell': [it depends on v0.20 of `go.opentelemetry.io/otel`](https://github.com/kubernetes/kubernetes/issues/106536). Preserving backwards-compatibility with older Kubernetes API objects also makes the update difficult.

This module is used to solve this dependency hell, by adding the `github.com/go-logr/logr/funcr` package into version v0.4.0 of the logr dependency.

It is the smallest patch that can be applied to solve this particular 'dependency hell' issue with logr. It will not fix other issues when packages depend on other, newer packages, or when packages depend on functions/structs not present on logr v0.4.0 (e.g. LogSink).

## When can it be removed?

This must be removed when logr is bumped to v1.0.0 or above.

## How does it work?

It bundles a copy of `github.com/go-logr/logr@v1.2.2` on the `internal/logr` folder, and uses a wrapper (see `internal/wrapper`) to make a v1.2.2 logger into a v0.4.0 logger (which, luckily, is an interface). Furthermore:

1. Non-public references to logr are replaced with references to the `internal/logr` copy. 
2. Public references are kept referencing v0.4.0 logr.
3. The `New` and `NewJSON` functions are rewritten using the wrapper.
