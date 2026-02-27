# Patterns

This package is useful to tokenize and cluster logs into patterns.
This is done in a concurrent way with multithreadpipeline.

## Pipeline overview

Incoming log messages are dispatched randomly across N tokenizers. After tokenization, messages are deterministically routed to a pattern clusterer based on `len(tokens) % N`, so messages with the same token count always land on the same clusterer — no locking needed.

```
                  Tokenizers                    PatternClusterers
               (random assignment)           (routed by len(tokens) % N)
                ┌─────────────┐                 ┌──────────────────────┐
                │ Tokenizer 0 │ ─────────────── │ PatternClusterer 0   │ ──┐
                ├─────────────┤                 ├──────────────────────┤   │
Log message ──► │ Tokenizer 1 │ ─────────────── │ PatternClusterer 1   │ ──┼──► ResultChannel
                ├─────────────┤                 ├──────────────────────┤   │
                │ Tokenizer N │ ─────────────── │ PatternClusterer N   │ ──┘
                └─────────────┘                 └──────────────────────┘
```

## Clustering logic (per PatternClusterer)

Each `PatternClusterer` uses a two-level lookup. It first looks for a compatible cluster in the same signature group, then falls back to all other groups to handle minor structural differences (e.g. a trailing `?` in a URL path). If no compatible cluster is found a new one is created.

Two token lists are compatible when they have the same length, all token pairs are type-compatible, and at least half of the tokens share the same value (Drain-style similarity threshold).

```
message
   │
   ▼
Tokenize → compute signature
   │
   ▼
Mergeable cluster in same signature group? ──yes──► Merge tokens        ──► ClusterResult (IsNew=false)
   │                                                 (wildcard diffs)
   no
   │
   ▼
Mergeable cluster in other signature groups? ──yes──► Merge tokens      ──► ClusterResult (IsNew=false)
   │                                                   (wildcard diffs)
   no
   │
   ▼
Create new Cluster                                                      ──► ClusterResult (IsNew=true)
```
