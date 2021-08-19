# Collisions test for the context key generator

## Automatic

Run `run_test.sh` after having read its sources.
Note that it runs for 17 minutes on a 2018 MacBook Pro i7 quad-core and needs more
than 3GiB of RAM.

## Manually

Use `generate_contexts.py` to generate random entries in a file called random_contexts.csv
Each line will have the format:

```
<metric_name>,<tag1> <tag2> ...
```

where the number of tags is random between 1 and 5.
All values (metric name and tags) will be random UUID4.

Once this file has been generated, make sure there is no duplicate contexts in there:

```
cat random_contexts.csv|sort|uniq > random_sorted_uniq_contexts.csv
```

It will run for several minute. Finally, run the unit test:

```
$ pwd
datadog-agent/pkg/aggregator/ckey/tests
$ go test ./... -run TestCollisions
```

If an error happen, it means that it has detected a collision.

This test will use a _lot_ of RAM (3GiB on my machine)