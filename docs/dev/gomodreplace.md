# How to add a `replace` directive to `go.mod`

`go.mod` files allow different types of directives including `require` or `replace`.
This document describes how and when to add a `replace` directive to our go.mod files.

## Why we should avoid `replace` directives

`replace` directives replace a given dependency code on the whole dependency tree; they were designed [to test unpublished code](https://golang.org/doc/modules/managing-dependencies#unpublished) although they are also used for nested modules or to fix broken dependencies. 

Using `replace` directives has a number of undesirable consequences:
1. The dependency is replaced by the fork/version everywhere in the dependency graph, including for other dependencies. Imagine the datadog-agent has dependencies B and C, and B depends on C. If we replace module C by module C’, we will change it also for B, which might have unintended consequences.
2. The dependency can’t be managed by Dependabot, so we are not notified when new versions come up.
3. We can be pointing to a no longer ‘existing’ version of a dependency (for example, referring to a commit that was deleted after a force-push) and we won’t notice while it stays on Github cache, breaking at an unexpected moment.
4. Consumers of the module can’t import it without replacing the dependency on their go.mod file too, since replace directives only affect the current module. This affects the creation and maintenance of new nested Go modules.

Because of this, we want to reduce the number of `replace` dependencies to the minimum possible, and document why the remaining dependencies need to be replaced.

## Alternatives to `replace` directives

Some uses of `replace` directives have better alternatives. We should use the alternatives if it is feasible to do so.

- **Use a fork.** Specify the fork path explicitly rather than via `replace`. That is, use `require github.com/DataDog/viper vX.Y.Z` rather than `replace github.com/spf13/viper => github.com/Datadog/viper vX.Y.Z`
- **Exclude a dependency version.**  If a single version is buggy, use an `exclude` directive or require a higher version of the dependency.

## Adding a new `replace` directive

Despite the above, it is sometimes necessary to add `replace` directives, either temporarily or permanently.

If you need to use a `replace` directive, add a comment above it explaining why you need it.
For example,

```
// etcd uses unstable code only available until grpc v1.26.0
// See https://github.com/etcd-io/etcd/issues/11563
replace (
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.12.2
	google.golang.org/grpc => github.com/grpc/grpc-go v1.26.0
)
```
