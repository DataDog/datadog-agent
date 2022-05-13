# `google.golang.org/grpc/credentials/insecure` patch

## What is this?

This is a copy of the contents of the `google.golang.org/grpc/credentials/insecure` package as of [commit db9fdf706d400bfc4d54665e1f06e863ed407f45](https://github.com/grpc/grpc-go/blob/9cb411380883ddbf69467b4ba1099817c0fe6c61/credentials/insecure/insecure.go).

It is exposed as a module and declares its path to be `google.golang.org/grpc/credentials/insecure`.

## What is the motivation behind this?

We need this because of a 'dependency hell' situation:

1. `grpc-go` makes breaking changes between minor releases of their module. 
2. etcd v3.5.0 [depends on a `grpc-go` API which was removed in v1.30](https://github.com/etcd-io/etcd/issues/12124). We depend on etcd v3.5.0 indirectly via Kubernetes v0.21.5.
3. `go.opentelemetry.io/collector` v0.42.0 and above depends on the `google.golang.org/grpc/credentials/insecure` package, which was added in v1.34. We depend on the Collector dependency directly.

This situation would be solved by bumping Kubernetes to v0.22.0 or above, which depends on etcd v3.6.0+, which does not make use of the removed grpc-go API.
However, Kubernetes can't be upgraded above v0.21.x, because of another 'dependency hell': [it depends on v0.20 of `go.opentelemetry.io/otel`](https://github.com/kubernetes/kubernetes/issues/106536).Preserving backwards-compatibility with older Kubernetes API objects also makes the update difficult.


This module is used to solve this dependency hell, by adding the `google.golang.org/grpc/credentials/insecure` package into version v1.28 of grpc-go.

It is the smallest patch that can be applied to solve this particular 'dependency hell' issue with grpc-go. It will not fix other issues when packages depend on other, newer packages.

## When can it be removed?

This must be removed when grpc-go is bumped above v1.34.0.
