## Hash selection

This documentation is intended for Agent developers.

### Collisions

Besides performance, an important consideration during the selection of a hash function is
how often it creates collisions: a collision in a function is when two different
inputs give the same output.

Datadog stores samples of the Agent aggregator in a map. In order to have smaller keys
while storing the values in this map, Datadog uses a hash of some identifying parts
of the sample: for instance, the metric name and its tags.
Thus, the map is used as a hashmap.

The Java HashMap implementation contains a "collision resolution" algorithm: when
two keys generate the same hash, because the HashMap stores values in buckets, it
can test the different entries behind the same hash and return the proper value.
This does not exist in the aggregator: while writing to the map, it is unknown if
the value form another sample is being overwritten. For the aggregator, it is important
to avoid collisions as much as possible.


### Birthday problem

You can compute the probability of having a collision using the Birthday Problem
(also known as the Birthday Paradox). The probability is dependent on the number
of different keys in the input, as well as the size of the output. For more
information, see the [Wikipedia article](https://en.wikipedia.org/wiki/Birthday_problem).
on the Birthday Problem.

With 64 bits, there is a very low probability of collision:

```
import math
probability = 1.0 - math.exp( (-k*(k-1))/float(2**n) )
```
From: https://en.wikipedia.org/wiki/Birthday_problem#Cast_as_a_collision_problem

with k = 2500000 being the number of different contexts and n = 64 bits (size of
the desired output), there is a chance of collision of approximately:

    3.388129860004696e-07

This is the number for a perfectly uniform hash. To ensure the quality of a hash,
you can refer to [smhasher](https://github.com/rurban/smhasher) to get various
quality performances test, for our case with our implementation
of Murmur3: https://github.com/rurban/smhasher/blob/master/doc/Murmur3F.txt

We see that the [avalanche effect](https://en.wikipedia.org/wiki/Avalanche_effect) is good
(in short, that two really similar inputs give two very different hashes) and
that it performs well on collisions tests. [xxhash64](https://github.com/rurban/smhasher/blob/master/doc/xxHash64.txt) is in the same situation.

In contrast, [FNV1A](https://github.com/rurban/smhasher/blob/master/doc/FNV1a.txt)
gives poor results.

## Raw performances

In terms of raw performances, there was no huge differences between the 3. I've
runned multiple long benchmarks on each of them and the hash algorithms themselves
were not making a huge difference between eachother.

I've decided to stick with Murmur3 (and to not switch to xxhash64) for two reasons:

    - we already ship an implementation of murmur3 in the Agent
    - the Go compiler doesn't behave the same while compiling with murmur3 & xxhash64; see below.

## 64 bits map keys and the Go runtime

The whole purpose of [my PR](https://github.com/DataDog/datadog-agent/pull/5209)
was to switch to 64 bits key in the maps of the samplers of the Agent. Why? Because
the Go runtime uses different methods while accessing a map with 64 bits integer keys
than with other kind of keys. It does the same while assigning value to the map.
See for instance
[runtime.mapassign_fast64 or runtime.mapaccess2_fast64](https://golang.org/src/runtime/map_fast64.go).

Benchmarks [available in the PR description](https://github.com/DataDog/datadog-agent/pull/5209)
show that the time sampler will have +36% performance improvement while using
64 bits keys instead of 128 bits keys.

On top of that, I've looked at debug profiles when using murmur3 64 bits hash
generation and xxhash64 bits generation: murmur3 has slightly better raw
performances because the Go compiler is inlining part of the hashing, which it
doesn't do for xxhash.
