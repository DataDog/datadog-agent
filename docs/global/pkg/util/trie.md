> **TL;DR:** Provides a generic in-memory `SuffixTrie` for O(path-length) longest-suffix match lookups, used to resolve container IDs from cgroup paths without a full linear scan.

# pkg/util/trie

## Purpose

`pkg/util/trie` provides a generic, in-memory `SuffixTrie` data structure. It stores values indexed by string suffixes and supports efficient longest-prefix (shortest-suffix) match lookups. The primary motivation is fast suffix matching on cgroup paths to resolve container IDs without full regular-expression scanning of every path.

## Key elements

### Key types

#### `SuffixTrie[T any]` (`trie.go`)

```go
type SuffixTrie[T any] struct { ... }
```

A trie whose keys are stored in reverse so that traversal walks from the end of the query string toward its beginning. `Get` returns the value for the *shortest* stored suffix that matches the end of the query.

| Function / Method | Description |
|---|---|
| `NewSuffixTrie[T]() *SuffixTrie[T]` | Creates an empty trie. |
| `(*SuffixTrie[T]).Insert(suffix string, value *T)` | Stores `value` under `suffix`. Empty strings are ignored (an empty suffix would match everything). |
| `(*SuffixTrie[T]).Get(key string) (*T, bool)` | Returns the value for the first (shortest) suffix that is a suffix of `key`. Returns `nil, false` if no suffix matches. |
| `(*SuffixTrie[T]).Delete(suffix string)` | Removes the entry for `suffix` from the trie; prunes orphan nodes. |

**Match semantics example:** if the trie contains `"foo"` and `"foobar"`, then `Get("foobarbaz")` returns the value for `"foo"` (shortest match).

## Usage

`SuffixTrie` is used by the container metrics subsystem on Linux:

`pkg/util/containers/metrics/system/filter_container.go` maintains a `*SuffixTrie[string]` that maps cgroup path suffixes to container IDs. When a cgroup path arrives that does not match the fast regex, it is looked up in the trie. Trie entries are populated and pruned in response to `workloadmeta` container set/unset events.

```go
// Example from filter_container.go
trie := trie.NewSuffixTrie[string]()
id := "abc123"
trie.Insert("/kubepods/pod.../abc123", &id)

containerID, ok := trie.Get("/sys/fs/cgroup/kubepods/pod.../abc123/cgroup.procs")
```

### Why suffix matching

Cgroup paths used in Kubernetes are hierarchical strings that end with the container ID segment
(e.g. `/kubepods/burstable/pod<uid>/<container-id>`). The agent needs to map an arbitrary
sub-path under that hierarchy (such as a cgroup control file) back to the owning container.
A suffix trie allows this in O(path length) time without scanning all known containers on each
lookup, which would be O(n × path length) with a linear search.

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/containers` | [containers.md](containers.md) | `metrics/system/filter_container.go` (part of the `metrics/system` collector within `pkg/util/containers`) owns the `SuffixTrie` instance and populates / prunes it based on `workloadmeta` `KindContainer` events. The trie is consulted as a fallback when the primary regex-based cgroup path matcher cannot find a container ID. `MetaCollector.GetContainerIDForInode` and `GetContainerIDForPID` are the public entry points that trigger this lookup path. |
