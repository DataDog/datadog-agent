# YOUR TITLE HERE

- Authors: your names here
- Date: 2000-01-01
- Status: draft|review|accepted|closed
- [Discussion](https://github.com/DataDog/datadog-agent/pull/0)

## Overview

*A paragraph concisely describing the problem you need to solve and and your
recommended solution. A tl/dr as the kids say. For example:*

The Agent should be able to run checks written in Python. This is a proposal to
embed CPython using `cgo`, so checks would run within the same process as the
Agent itself.

## Problem

*If necessary, add a more detailed sketch of the problem. Add any notes,
pictures, text or details that illuminate why this is a problem we need to
solve. Keep it as concise as possible.*

## Constraints

*If necessary, note any constraints or requirements that any solution must
fulfill. For example:*

    1. We have to target Python 2.7.
    2. Memory footprint should not increase more than 10%.
    3. Every check in `integrations-core` and `integrations-extra` should be
       supported.

## Recommended Solution

*Enumerate possible solutions to the problem. Note your recommended
solution first. For example:*

### Embed CPython 2.7 using cgo

*Describe your solution in the bare minimum detail to be understood. Explain
why it's better than what we have and better than other options. Address
any critical operational issues (failure modes, failover, redudancy, performance, cost).
For example:*

Embedding CPython is a well known, documented and supported practice which is quite
common for C applications. We can leverage the same C api using cgo...

- Strengths
    - easily share memory between Go and Python
- Weaknesses
    - adapt Go's concurrency model to Python threading execution model might cause
      issues
    - we can't cross compile anymore
    - a Python crash would bring down the Agent
- Performance
    - ...
- Cost
    - we add about 4Mb of memory footprint
    - ...

## Other Solutions

*Describe a few other options to solve problem.*

###  Spawn a Python process for each check and use gRPC to communicate

*Jot a few nots on other things you've considered.*
At each collection cycle the Agent would fork a Python process running a check...

## Open Questions

*Note any big questions that don’t yet have an answer that might be relevant.*

## Appendix

*Link any relevant stats, code, images, whatever that work for you.*
