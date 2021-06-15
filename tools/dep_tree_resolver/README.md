# Dependency Tree Resolver

For both security and legal needs, we need to be able to generate a full list
of dependencies for our codebase that includes the actual versions used (vs the
ones specified) when the code is being run. While no method is perfect at this
task, we can use this helper script to generate our best-guess at the resulting
dependency tree.

_Note that this program only calculates the output based on the content of the main
`go.mod` and its dependencies and it does not handle depndencies declared in a
other isolated modules (e.g.
[`tools.go`](https://github.com/DataDog/datadog-agent/blob/master/internal/tools/tools.go)._

## Usage

1. Ensure that you have a version of Golang that can handle modules (though having
   the exact version from [go.mod](https://github.com/DataDog/datadog-agent/blob/master/go.mod#L3)
   is preferrable).
2. Ensure that you are in the root of the desired project
3. Run the script:
   ```sh-session
   $ go run tools/dep_tree_resolver/go_deps.go

   (  1/662) cloud.google.com/go/pubsub -> {cloud.google.com/go/pubsub v1.3.1}
   (  2/662) github.com/containerd/zfs -> {github.com/containerd/zfs v0.0.0-20210315114300-dde8f0fda960}
   ...
   Computing actual dependency tree (this will take a while)...
   ...
   Writing output to 'dependency_tree.txt' (this may take a while)...
   Done!
   ```

Output will be created in a file (filename will be printed by the script) within the
same directory.

## Process

The logic to generate this output is based on how Golang assembles the dependencies
but due to the lack of good built-in tooling we have to do some of the processing
ourselves.

Steps taken (with explanation where needed):

- The dependency tree is initially parsed from output of `go mod graph`. This output
  is useful to understand the high-level linking of modules (`<ID>@<VERSION>`) to its
  dependencies.
- The list of all module IDs is resolved sequentially to get the version that will actually
  be used when the program gets built. While the `go mod graph` output may contain many
  versions of the same module ID,
  [only a single version](https://github.com/golang/go/wiki/Modules#version-selection) will
  be resolved in the end and we go through each of the module IDs to find that version.
  The reason why the operation is done sequentially and via CLI is that all helpers are
  private and `go mod list -m` CLI performs a lock during evaluation so parallel execution
  is useless.
- After the real versions are evaluated, we follow the dependency chain recursively,
  replacing all `<ID>@<VERSION>` dependencies with the actual versions and their
  dependencies resolved from the previous step. This both fixes the version numbers and
  corrects the dependencies that may change as the version is adjusted.
- During this process, we also keep track of cyclic dependencies and break out of the
  recursive resolution if we find ourselves in a loop. Sadly, some base Golang packages
  have this issue.
- After recalculating the whole tree, we write it to a file though a small modification to
  the code can change the output to be printed to stdout.
