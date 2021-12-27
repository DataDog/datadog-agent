# Unified Handling of Tagsets in the Agent

- Authors: Dustin Mitchell
- Date: 2021-10-22
- Status: **draft**|review|accepted|closed
- [Discussion](https://github.com/DataDog/architecture/pull/0)

## Overview

Most of the data handled by the agent has tags attached, so a substantial portion of the agent's code handles tags.
This RFC proposes to unify handling of sets of tags (tagsets) with an API that is safe, performant, and easy to use.

The proposed API relies on an immutable type to ensure safety.
it is designed to make common operations on tagsets, such as parsing, performing unions, or adding additional tags, easy and natural.
The high-level API is sufficiently general to support performance optimization (both CPU and memory usage) "under the hood", independent of the users of the API.
Some performance improvements are included in this proposal, but measurement may help us uncover further opportunities for improvement once these changes are deployed.

## Problem

_Safety:_ Within the agent, tags are handled with type `[]string`, and are frequently mis-used in an unsafe way.
For example, it is common to receive a slice of tags and use the `append(..)` operator to add tags to it.
When the original slice of tags is used by multiple components (for example, if it is the cached set of tags for a container), such append operations may overwrite each other, resulting in incorrect or missing tags.
Such safety issues have led to customer support cases that are difficult to diagnose and even more difficult to fix.

_Performance:_ The DogStatsD component often handles extremely high volumes of metrics, and has recently been the focus of performance work.
Tags for DogStatsD metrics must be parsed, aggregated, and serialized for sending to the intake.
The aggregation process involves hashing the tags in order to aggregate metrics points with the same tags together.
All of this comprises a large part of DogStatsD's memory and CPU footprint.

As a general observation, there is a "hot set" of tagsets that are used repeatedly, on the scale of a few seconds.
That "hot set" changes comparatively slowly.
This proposal aims to capture and handle that hot set efficiently, storing it once and avoiding repeated calculations.

Other agent components are less performance-sensitive, but still benefit from more efficient handling of tags, leading to more consistent, lower usage of customer resources.

_Ease of Use:_ It takes a great deal of attention from engineers to handle tags safely.
As an example, simply adding caching to a performance-sensitive bit of tag-related code may result in incorrect or missing tags.
Worse, such issues are unlikely to be caught in the simple environment of unit tests or in review, and will only be caught in release QA or, worse, in customers' deployments.

Aside from safety issues, tags currently require more engineer head-space than they should: is that `[]string` sorted? are the contents unique?

## Constraints

This solution must not introduce additional instability into the agent.

It must also be deployed gradually and in such a way that teams responsible for each agent component can adopt it as their schedule allows.

## Recommended Solution

The recommended solution is broken down into four areas:

 * API - describes the "outward-facing" portion of the new API, as it will be seen by other agent engineers
 * API Implementation - describes the implementation of the new API, including some performance optimizations
 * Implementation Process - describes how this will be implemented within and among the agent teams
 * Analysis - analysis of the proposed solution (strengths, weaknesses, failure modes, security, etc.)

### API

The API is agnostic to the content of tags, treating them as opaque strings.
The exception is that some methods treat tags as colon-separated `key:value` pairs for convenience.

#### Tags

The Tags type is an opaque, immutable data structure representing a set of tags.
Agent code that handles tags, but does not manipulate them, need only use this type.
It has the following methods:

 * Representations - _NOTE:_ order is not defined for any of these methods
   * `Tags.String() string` - human-readable string form
   * `Tags.MarshalDSD() []byte` - serialize in DogStatsD format (comma-separated)
   * `Tags.MarshalJSON() ([]byte, error)`
   * `Tags.MarshalYAML() ([]byte, error)`
 * Queries
   * `UnsafeSliceToTags([]string) *Tags` - temporary constructor for use during phase 2 -- to be removed before phase 3.
   * `Tags.Hash() uint64` - the hash of this set of tags
   * `Tags.Sorted() []string` - a sorted _copy_ of the contained slice of strings (intended for testing)
   * `Tags.Contains(tag string) bool`
   * `Tags.IsSubsetOf(tags *Tags) bool`
   * `Tags.WithKey(key string) []string` - finds all tags with the given key
   * `Tags.FindByKey(key string) string` - finds the first tag with the given key
   * `Tags.ForEach(each func (tag string))` - calls `each` for each tag
   * `Tags.UnsafeReadOnlySlice() []string` - get a read-only slice of strings; use of this interface should be minimal, but it's necessary to interface with things like Prometheus or Datadog-Go.
     The docstrings for this method will instruct callers to add a comment explaining why the use is safe.

#### Factory

The Factory type is responsible for creating new Tags instances.
Its interface is simple, but provides an opportunity for optimization and deduplication.

As with many Go packages, a single default factory is provided for use throughout the agent, with package-level functions deferring to that factory.
Additional, specific factories may be created for specific purposes.
The default factory is thread-safe, but it may be advantageous to build non-thread-safe factories for specific circumstances.
Tags instances created by different factories can be used interchangeably and are entirely thread-safe.

 * Constructors
   * `Factory.NewTags(src []string) *Tags` - create a new Tags, leaving ownership of the slice with the caller (so modifying the slice after calling this method will not modify the resulting Tags instance)
   * `Factory.NewTagsFromMap(src map[string]struct{}) *Tags` - create a new Tags based on the keys of the given map (also implying uniqueness)
   * `Factory.NewTag(tag string) *Tags` - create a new Tags, containing only the given tag
   * `Factory.NewBuilder(capacity int) Builder` - create a new Builder, tied back to this factory for caching purposes
   * `Factory.NewSliceBuilder(levels, capacity int) SliceBuilder` - create a new SliceBuilder, tied back to this factory for caching purposes
 * Parsing
   * `Factory.UnmarshalJSON(data []byte) (Tags, error)`
   * `Factory.UnmarshalYAML(data []byte) (Tags, error)`
   * `Factory.ParseDSD(data []byte) (Tags, error)` - Parse tags in DogStatsD format (comma-separated)
 * Combination
   * `Factory.Union(a, b Tags) *Tags` - perform a union of two Tags instances, which may contain the same tags. *How Will This Be Used?* See "Usage Examples", below.
   * `Factory.DisjointUnion(a, b Tags) *Tags` - like Union, but with the caller promising that the sets are disjoint

#### Builders

Builders are used to build tagsets tag-by-tag, before "freezing" into one or more `Tags` instances.
Builders are not threadsafe.

A builder goes through three stages in its lifecycle: 
 1. adding tags (begins on `factory.New*Builder`).
 2. frozen (begins on call to `acc.Freeze*`).
 3. closed (begins on call to `acc.Close`).

The `Add` methods may only be called in the first stage.
No methods may be called in the third stage.

The Builder type supports the common use-case of conditionally adding a handful of tags to a set.
Adding the same tag twice to an Builder is a no-op.

The SliceBuilder type supports creation of overlapping "slices" of tags.
This supports Tagger's tag-cardinality functionality while saving some memory space.
For example, a SliceBuilder can produce a Tags containing high, orchestrator, and low-cardinality tags as well as a Tags with only the low-cardinality tags, sharing that space.
Adding the same tag twice to a SliceBuilder is a no-op if both are at the same level, but results in undefined behavior if the levels differ.

*How Will This Be Used?* SliceBuilder will be used in the Tagger, which calculates all tags for an entity and then slices those tags by cardinality.
See "Usage Examples", below.

Builder methods are:

 * `Builder`
   * `Builder.Add(tag)` - add a tag
   * `Builder.AddKV(key, value)` - add a tag in key:value format
   * `Builder.Contains(tag) bool` - returns true if the tag is in the builder
   * `Builder.Freeze() *Tags` - return a Tags containing the added tags
   * `Builder.Close()`
 * `SliceBuilder`
   * `SliceBuilder.Add(level, tag)` - add a tag to the given level
   * `SliceBuilder.AddKV(level, key, value)` - add a tag to the given level in key:value format
   * `SliceBuilder.Level(tag) int` - if the tag is already in the builder, returns its level, else -1
   * `SliceBuilder.FreezeSlice(a, b int) *Tags` - return a Tags containing levels `[a:b]`
   * `SliceBuilder.Close()`

#### Usage Examples

The following are a few examples of how this functionality might be used in practice.

##### Union

Consider a DSD metric from intake to serialization: the DSD worker parses the incoming message into a metric with Tags attached, and passes that along to the aggregator.
The aggregator calls the Tagger to get enriching tags.
The aggregator then uses a factory's `Union` operation to combine the parsed and enriching tags into the final Tags for this metric.
It calculates the context hash, using the already-calculated `Tags.Hash()` for the tags, and aggregates appropriately.

```go
func (m *MetricSample) GetTags() *tagset.Tags {
    return tagset.Union(m.Tags, tagger.OriginTags(m.OriginID, m.K8sOriginID, m.Cardinality))
}
```

##### SliceBuilder

When the Tagger learns of a new entity, it uses a `SliceBuilder` to accumulate tags by cardinality, mapping each cardinality to a level.
It then uses the `FreezeSlice` methods of that builder to generate slices at each possible cardinality.
These slices share a single backing array, saving memory.
The Tagger code is too complex to include here, but the following snippet demonstrates the concept:

```go
bldr := tagset.NewSliceBuilder(NumCardinalities, 10)  // NumCardinalities levels, 10 tags each
for _, entityTag in entity.tagsAndCardinalities {
    bldr.Add(entityTag.Cardinality, entityTag.Tag)
}
entity.LowCardTags =bldr.FreezeSlice(0, LowCardinality+1)
entity.LowOrchCardTags =bldr.FreezeSlice(0, OrchestratorCardinality+1)
entity.AllTags = bldr.FreezeSlice(0, NumCardinalities)
bldr.Close()
```

### API Implementation

In general, the implementation aims to reduce memory usage through interning and deduplication, and to reduce CPU usage through caching and elimination of redundant work.

The current performance hot-spots related to tags are:
 * hashing tagsets in the metrics aggregator (especially in high-bandwidth DogStatsD deployments);
 * parsing tagsets from DogStatsD listeners; and
 * interning strings.

#### Tags

Internally, tags are stored in `[]string`.
64-bit murmur3 hashes are generated for tags as soon as they are created and stored in a parallel `[]uint64`.
All other operations -- sorting, verifying uniqueness, and so on -- are performed on the hashes, eliminating the need to re-scan the given strings.

Each Tags instance has a hash which is the XOR of the hashes of the tags it contains.
This has some attractive properties:
 * It is independent of order.
 * It maintains the entropy of the input hashes.
 * Disjoint tag sets' hashes can be combined with a single XOR.

The computatational cost of hashing is that tags must be de-duplicated in order to ensure a correct hash.
De-duplication is also attractive for memory usage and serialization.
Internally, Tags instances are *always* de-duplicated, containing only one copy of each tag.

#### Builders

Builders are typically used to establish small sets of tags, and are a mutable, non-threadsafe data structure.
They do not tend to be in performance-critical codepaths, so their implementation will be relatively simple.

Builders' `Close` methods support reuse of builders via a `sync.Pool`, minimizing unnecessary reallocations.

#### Factories

Factories implement caching of tagsets, reducing memory usage through de-duplication and saving CPU time by avoiding redundant hashing and allocations.
Factories use interfaces and composition internally to support a variety of configurations.
For example, a threadsafe factory is implemented as a locking wrapper around an un-threadsafe factory.

Each top-level factory maintains a set of caches, each for a different purpose, mapping `uint64` to `*Tags`.
The caches are used to cache different kinds of data.
For example, when parsing a comma-separated list, `Factory.ParseDSD` begins by hashing the input string and searching for an existing Tags instance in a dedicated cache.
Serializations, unions, and so on are similarly cached.

Access to these caches is implemented with an internal `Factory.GetCachedTags(cacheId CacheID, key uint64, miss func() *Tags) *Tags` which takes an integer CacheID identifying the cache to query.
If the item is not available in the cache, then the `miss` function will be used to generate the item.
Cache IDs will be statically allocated small integers.

Caches also use composition.
At the lowest level, the null cache caches nothing and always misses.
Composed on top of that is an interning cache which caches values forever, falling back to the null cache on misses.

At the top level is a revolving cache, which keeps an array of interning caches from oldest to newest, and tries them in order.
When an interning cache after the first returns a cache hit, the entry is copied to the first interning cache.
The array of caches is rotated periodically, with entries in the discarded cache becoming unreachable.

The result is a kind of multi-level LRU cache that can be tuned to optimize memory or CPU usage.
The tuning might be different for different types of caches.
Later improvements may perform such tuning automatically, all safely hidden behind the tagset API.

### Analysis

- Strengths
  - Safe handling of tags will reduce customer support cases and release regressions
  - Performant tagsets will reduce resource usage and improve throughput for high-volume customers
  - A robust, easy-to-use interface will accelerate development of future agent functionality

- Weaknesses
  - As a large-scale refactor, this change is likely to introduce new bugs (but good software engineering practices can catch those early)
  - There is no practical way to gate this change behind a feature flag
  - By changing how tags are handled (rather than simply optimizing the existing approach), this change may introduce performance regressions in circumstances we do not anticipate
  - This refactor requires interaction with many agent teams, some of which may have limited capacity to act.

- Failure Modes
  - The most likely failure mode is a critical bug that goes undetected through the release QA process and affects customers.
    Downgrading from the affected version would fix the issue in the short term, with a bugfix release to follow up.
    This is the same failure mode as for any large-scale change to the agent.
    It can be mitigated somewhat by spacing changes over several releases.

- Performance
  - Performance is a focus of this proposal.

- Cost & efficiency
  - Increased performance should marginally decrease overall costs for benchmark, QA, alpha, beta, staging, and production environments.

- Security
  - This is not a security-relevant proposal.

## Other Solutions

This proposal has the most "room for change" in the details of the recommended solution, rather than other solutions.

### Fix Safety Issues As They Are Found

In this solution, we address only the safety issues by patching them as they are discovered.
In most cases, such patches entail copying the slice of tags, which can have adverse performance impacts.

It may be possible to develop static-code analysis tools which could identify unsafe use of tags on engineers' systems or in CI, which would allow fixing existing issue and avoiding introduction of new bugs.

## Open Questions

* Q: Is a 64-bit hash sufficient for the purposes used here?
  Murmur3 supports 128-bit hashes (at additional CPU cost), and the various hash tables used throughout the implementation could be extended to use 128-bit hashes using two-choice hashing (again at additional CPU cost).

* Q: Should we use a string interner for tags?
  Interning at the tagset level may make this unnecessary.

## Appendix

 * [dogstatsd-tags-lists-cache RFC](../dogstatsd-tags-lists-cache/rfc.md)
 * [tagsets experiments](https://github.com/djmitche/tagset-experiment)
 * [phase 1 in-progress](https://github.com/DataDog/datadog-agent/compare/dustin.mitchell/tagset-impl)
