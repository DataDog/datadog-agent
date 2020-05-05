## Hash selection

This documentation is intended for the Agent developers.

### Collisions

Besides performances, what is very important during the selection of a hash is
how often it is creating collisions: a collision in a function is when two different
inputs give the same output.

We are storing samples of the Agent aggregator in a map: in order to have smaller keys
while storing the values in this map, we are using a hash of some identifying parts
of the sample, for instance, the metric name and its tags.
Thus, we're using the map as a hashmap.

Something important to know is that for example in the Java HashMap implementation,
there is something else involved which is called a "Collision resolution" algorithm:
when two keys generates the same hash, because the HashMap is storing values in
buckets, it is capable of testing the different entries behind the same hash and
to return the proper value. This is something we don't have in the aggregator:
while writing to the map, we don't know if we're overriding the value from
another sample.

This is why collisions are important to avoid in our case and that the fewer
collisions, the better.

### Birthday problem

The probability of having two different keys resulting to the same hash could be computed
with formulas from the the birthday problem / birthday paradox and it is dependent
on the amount of different keys in input but also on the size of the output.

I won't paraphrase, here's the link to [a complete page on Wikipedia on the topic](https://en.wikipedia.org/wiki/Birthday_problem).

With 64 bits we have a very low probability of collision:

```
import math
probability = 1.0 - math.exp( (-k*(k-1))/float(2**n) )
```
From: https://en.wikipedia.org/wiki/Birthday_problem#Cast_as_a_collision_problem

with k = 2500000 being the number of different contexts and n = 64 bits (size of
the output I'm aiming for), we have a chance of collision of approximately:

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
    - the Go compiler doesn't behave the same while compiling with murmur3 & xxhash64, see below.

## 64 bits map keys and the Go runtime

The whole purpose of [my PR](https://github.com/DataDog/datadog-agent/pull/5209)
was to switch to 64 bits key in the maps of the samplers of the Agent? Why? Because
the Go runtime use different methods while accessing a map with 64 bits integer keys
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

