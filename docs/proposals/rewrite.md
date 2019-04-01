# Rewrite the Agent

- Authors: massi
- Date: 2019-04-01
- Status: draft
- [Discussion](https://github.com/DataDog/datadog-agent/pull/0)

## Overview

Agent 6 was a complete rewrite of 5 using a different programming language and
a different software architecture. It was fun to do, let's do it again.

## Problem

The real problem will be finding approval and resources for the project but let's
not think about this for a moment.

## Constraints

1. Solution must be packaged with Omnibus Software
2. Agent must run integrations in Go and Python

## Recommended Solution

### Use Rust

The Agent should be rewritten in Rust, this is a proposal for an incremental
refactoring that will let us swap Golang packages with Rust crates as we go.
New features will be written in Rust directly.

Golang is robust and mature, the learning curve is smooth but its hype is going
down [link](https://trends.google.com/trends/explore?date=2016-07-09%202019-01-04&geo=US&q=go) with the version 2.0 approaching and the introduction of generics,
arguing with strangers won't be fun anymore and trolling `r/golang` will become
harder and harder. Also, garbage collectors are clearly overrated.

Compatibility with Python will be kept by embedding a Go library built out of
the existing packages that embed the CPython interpreter using `cgo`.

- Strengths
    - Rust is safe
    - Rust is modern
    - Rust is performant
- Weaknesses
    - Couldn't find one honestly
- Cost
    - One team of 2 or 3 engineers working fulltime for 4 to 6 quarters

## Other Solutions

###  Use Crystal

- Strengths
    - fast as C, slick as Ruby
    - its Ruby-like syntax will make Omnibus specialists feel at home
- Weaknesses
    - hasn't hit 1.0 yet but it might have by the time we finish
- Cost
    - negligible if we think we'd help pushing Crystal through 1.0

###  Use Kotlin

- Strengths
    - better than Java
- Weaknesses
    - fully interoperable with Java
- Cost
    - we'd pay in engineers retention, not money

## Open Questions

*Note any big questions that don’t yet have an answer that might be relevant.*
