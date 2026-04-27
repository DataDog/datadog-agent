# Pattern mining — case matrix

What the tree does at **merge time** and at **anomaly-check time** for every
distinct scenario we've identified. Two proposed changes are being
evaluated; both are orthogonal and can ship independently.

- **R1 — Homogeneity rule (merge time).** A bare-wildcard template (`*`,
  `*-*`, …) is accepted only when every non-pattern sibling at this level
  shares the bucket's signature. Non-bare templates (with a literal anchor)
  are unaffected.
- **R2 — Signature-tagged lookup (anomaly time).** Pattern nodes carry the
  structure signature of their training members. `findChildWithPatternFallback`
  requires the candidate's signature to equal the pattern's before returning
  a hit.

Legend: ✓ merge/hit, ✗ refused/miss, → anomaly, 🔇 quiet (no anomaly).

## Merge-time cases

For each scenario, `minClusterSize = 3` (finalize pass).

### 1. Homogeneous numeric fan-out

```
/var/log/job/1  /var/log/job/42  /var/log/job/1337  /var/log/job/99999
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `N` | all 4 | `*` | ✓ merge | ✓ merge | only one signature in the map, bare `*` safe |

### 2. Homogeneous with literal anchor

```
/srv/sess-aaa  /srv/sess-bbb  /srv/sess-ccc
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `A-A` | all 3 | `sess-*` | ✓ merge | ✓ merge | non-bare, literal anchor protects precision |

### 3. Heterogeneous, bare-wildcard bucket in the mix

```
/tmp/filename  /tmp/1  /tmp/42  /tmp/1337  /tmp/99999
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `A` | `filename` | — | skip (size 1) | skip | below threshold |
| `N` | 4 numerics | `*` | ✓ merge → `*` | **✗ refuse** | map also contains `A` sibling; bare `*` would absorb alpha names too |

### 4. Heterogeneous with literal-anchored bucket present

```
/sub/filename  /sub/412422  /sub/4353535  /sub/63634646  /sub/323526
       /sub/prefix-32424  /sub/prefix-525252  /sub/prefix-335323
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `A` | `filename` | — | skip | skip | below threshold |
| `A-N` | 3 `prefix-*` | `prefix-*` | ✓ merge | ✓ merge | non-bare, literal anchor |
| `N` | 4 numerics | `*` | ✓ merge → `*` | **✗ refuse** | `filename` (sig `A`) still present → not homogeneous |

Final tree under R1:

```
sub
├── filename
├── 412422, 4353535, 63634646, 323526   (all literal)
└── prefix-*                             (pattern, sig A-N)
```

### 5. Fixed-alpha top-level directories (existing guard)

```
/tmp  /etc  /var  /bin  /usr  /home
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `A` | 6 fixed names | `*` | ✗ refuse | ✗ refuse | existing `signatureHasVariableClass` guard — already handled |

### 6. Two bare-wildcard buckets coexisting

```
/dir/1 /dir/42 /dir/1337 /dir/99999          ← 4× signature N
/dir/abc123  /dir/def456  /dir/xyz789        ← 3× signature M
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `N` | 4 numerics | `*` | ✓ merge | **✗ refuse** | `M` siblings differ in signature |
| `M` | 3 mixed | `*` | ✓ merge | **✗ refuse** | `N` siblings differ in signature |

(If this case becomes common in production, variant #3 — "allow when all
other signatures are variable-class" — can be considered. For now, R1
prioritizes correctness over compression.)

### 7. Pattern node already exists in the map

```
/var/log/<once merged: "2024-*">   ← IsPattern=true, sig N-N-N.A
/var/log/backup-1  /var/log/backup-2  /var/log/backup-3
```

| Bucket | Members | Template | Today | R1 decision | Rationale |
|---|---|---|---|---|---|
| `A-N` | 3 `backup-N` | `backup-*` | ✓ merge | ✓ merge | non-bare, R1 irrelevant; pattern siblings are ignored by the homogeneity check in any case |

## Anomaly-check cases

The tree contains a pattern under `/tmp/1234/subfolder/*` (sig `N`, from 4
numeric training members) and a sibling pattern `prefix-*` (sig `A-N`).

### A. Variant of the training shape

```
insert /tmp/1234/subfolder/999999   (sig N)
```

| Today | R2 decision | Outcome |
|---|---|---|
| template `*` matches → 🔇 | template matches AND sig `N` == `N` → 🔇 | correct — learned variant |

### B. Unrelated alpha name at the same leaf

```
insert /tmp/1234/subfolder/malicious_binary   (sig A)
```

| Today | R2 decision | Outcome |
|---|---|---|
| template `*` matches → 🔇 (**missed anomaly**) | template matches but sig `A` ≠ `N` → ✗ → new entry → → | anomaly raised under R2 |

Under R1 alone (no pattern produced here in scenario 3/4): exact lookup
misses → new entry → → . Both rules converge to the right answer.

### C. Different shape with digits

```
insert /tmp/1234/subfolder/prefix-99    (sig A-N)
```

| Today | R2 decision | Outcome |
|---|---|---|
| `*` template matches → 🔇 (**missed anomaly**) | `*` matches template but sig `A-N` ≠ `N` → miss. `prefix-*` template matches AND sig `A-N` == `A-N` → 🔇 | correct — `prefix-*` is a learned shape |

### D. Absorbing different alpha variant into a literal-anchored pattern

```
insert /srv/sess-root    (sig A-A)     tree has pattern "sess-*" trained on A-N
```

| Today | R2 decision | Outcome |
|---|---|---|
| template `sess-*` matches → 🔇 (**questionable hit**) | template matches but sig `A-A` ≠ `A-N` → miss → new entry → → | anomaly raised under R2 — user can decide whether alpha root vs numeric session is "the same shape" |

### E. Path-traversal-ish artifact

```
insert /tmp/1234/subfolder/..    (weird name, sig "..")
```

| Today | R2 decision | Outcome |
|---|---|---|
| `*` template matches → 🔇 | template matches but stored sig `N` ≠ `..` → miss → → | anomaly raised under R2 |

Under R1 alone: no pattern exists at this leaf (scenario 3) → exact miss →
→ . Both rules converge.

### F. New ID variant on a non-heterogeneous tree

```
tree was homogeneous; pattern `*` (sig N) was merged.
insert /var/log/job/424242   (sig N)
```

| Today | R2 decision | Outcome |
|---|---|---|
| `*` matches → 🔇 | template matches AND sig `N` == `N` → 🔇 | correct — legitimate variant |

## Decision matrix summary

| # | Case | Today | R1 only | R1 + R2 |
|---|---|---|---|---|
| 1 | homogeneous numeric fan-out | merge, absorb variants 🔇 | merge, absorb 🔇 | merge, absorb correct-shape 🔇 |
| 2 | literal anchor | merge, absorb variants 🔇 | merge 🔇 | merge, absorb correct-shape 🔇; alpha variants → |
| 3 | heterogeneous bare-wildcard | merge into `*`, absorb alpha → (leak) 🔇 | no merge, `*` never exists → for all new names | same as R1 |
| 4 | heterogeneous, mixed literal + bare | as 3 plus `prefix-*` merges 🔇 for alpha-in-prefix | `prefix-*` merges, bare `*` refused | `prefix-*` merges with sig check |
| 5 | fixed alpha top-level dirs | refused (existing guard) | refused | refused |
| 6 | two bare-wildcard buckets | both merge to `*` (collision) | both refused | both refused |
| 7 | pattern already exists | unchanged | unchanged | pattern sig check preserves precision |

## What I recommend

- **Ship R1 now.** Tiny diff, closes the specific leak your example
  flagged, no protobuf changes, no schema migration.
- **Ship R2 as a follow-up** if D / E become concerns in practice. R2
  requires storing `PatternSignature` on the pattern `FileNode` and
  deciding whether to persist it across profile serialization (I'd
  say yes, to preserve precision across reloads — but that's a small
  proto change).

R1 alone already fixes the concrete concern (bare `*` at a leaf
absorbing alpha names). R2 adds defense for non-bare-template cases
like scenario D.

## Cost of R1 we're accepting

Mixed-parent directories that are currently heavily compressed (e.g.
`/tmp` containing both `.X11-unix` and thousands of PID subdirs) will
grow their numeric children as literals. The profile size cap remains
the backstop: once reached, new events are dropped with the existing
`event_dropped` metric — well-understood and already monitored. The
trade-off is "larger profile, sharper anomaly signal" instead of
"smaller profile, silent miss". Reasonable default for a security
product.
