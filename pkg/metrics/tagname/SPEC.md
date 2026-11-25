# Metric tag normalization: specification

Status: normative. This document and the vector files in `testdata/` together
define how the Datadog metrics intake normalizes a tag, and how the key of a
stored tag is derived. Character data comes from the Unicode Character Database
(§3.2, §3.3). Agent-side implementations, the Datadog Agent's
`pkg/metrics/tagname` and Agent Data Plane (ADP), must conform to it.

The key words MUST, MUST NOT and MAY are used as in RFC 2119.

## 1. Purpose and scope

The Agent sends metric tags verbatim. The intake normalizes them on ingest, and
users see, query and write rules against the normalized form. An Agent-side
decision that must agree with what the intake stores, such as matching
`metric_tag_filterlist` rules, has to apply the same normalization first.

This specification covers the intake's **standard mode** (`NormalizeTag` with
its default 200-byte limit). That is the mode the intake applies to:

| Payload | Mode |
| --- | --- |
| Sketches (distributions) | Always standard |
| Series (counts, gauges, rates), sent without `Allow-Arbitrary-Tag-Value: true` | Standard |
| Series sent with `Allow-Arbitrary-Tag-Value: true` | Arbitrary-tag-value mode: **not covered** (§11) |

The Agent does not send that header by default. The intake's long-tag mode
(5 KiB limit) is not reachable from the Agent and is not covered.

Out of scope: tag-set operations (sorting, de-duplication, dropping empty tags,
the tag-count and tag-set-size limits), host and device normalization, and
metric-name normalization (in the Datadog Agent, `pkg/metrics/metricname`).

## 2. Sources and precedence

When sources disagree, the higher one wins and the lower one is a bug:

1. The intake's behavior: dd-go `model.NormalizeTag`.
2. The vector files `testdata/vectors.txt` and `testdata/rule_vectors.txt`
   (§9). Every expected value was produced by dd-go.
3. This document.
4. Any implementation.

## 3. Definitions

### 3.1 Input

A tag is a finite sequence of bytes. It need not be valid UTF-8.

The input is decoded into a sequence of pairs *(offset, c)*, where *offset* is
the byte offset at which the code point *c* starts, as follows. At each offset:

- If the bytes starting there form a valid UTF-8 encoding of a Unicode scalar
  value, *c* is that scalar value and the next offset follows its encoding.
- Otherwise *c* is U+FFFD and the next offset is *offset + 1*. Each byte of an
  invalid sequence therefore decodes separately. Encoded surrogates
  (`ED A0 80` to `ED BF BF`), overlong forms and values above U+10FFFF are
  invalid.

This is the decoding of a Go `for range` over a string. U+FFFD itself is not a
Letter or Digit, so it is treated exactly like an invalid byte.

An implementation whose input is guaranteed to be valid UTF-8 (for example
because invalid bytes were already replaced upstream) MUST normalize the string
it is actually going to send, after any such replacement.

### 3.2 Character classes

- **Letter**: a code point whose Unicode `General_Category` is `L` (`Lu`, `Ll`,
  `Lt`, `Lm`, `Lo`).
- **Digit**: a code point whose Unicode `General_Category` is `Nd` only. Other
  numbers (`No`, such as `²`, and `Nl`, such as `Ⅳ`) are not Digits.

Combining marks (M) are neither: a decomposed `é` (`e` followed by U+0301) does
not normalize like the single code point `é` (U+00E9). No Unicode normalization (NFC or
otherwise) is applied.

Implementations MUST NOT substitute a different notion such as Unicode's
Alphabetic or Numeric properties.

### 3.3 Lowercase

**lower(c)** is the Unicode *simple* lowercase mapping (`Simple_Lowercase_Mapping`,
field 13 of `UnicodeData.txt`); code points without one map to themselves. It
maps one code point to exactly one code point, and may change the encoded length
in either direction. For example U+212A KELVIN SIGN (3 bytes) maps to `k`
(1 byte), and U+023A (2 bytes) maps to U+2C65 (3 bytes).

It is not the full mapping: U+0130 maps to `i` alone, not to `i` followed by
U+0307. It is not context-sensitive: capital sigma always maps to `σ`, never to
final `ς`. It is not case folding: `ß` and `ſ` are unchanged.

## 4. Tag normalization: N(t)

N(t) is defined by the following state machine. It is a single left-to-right
pass that never looks ahead.

**Constants.** `MaxLength` = 200. `MaxInputOffset` = 400.

**State.** `out`, a byte string, initially empty. `lastWasUnderscore`, a
boolean, initially false.

**Step.** For each decoded pair *(offset, c)* of the input, in order:

1. If `len(out) >= MaxLength`, stop.
2. If `offset > MaxInputOffset`, stop.
3. Apply the first matching branch:

   | Branch | Condition | Action |
   | --- | --- | --- |
   | B1 | `c` is in `a`–`z` | append `c`; `lastWasUnderscore` = false |
   | B2 | `c` is in `A`–`Z` | append lower(`c`); `lastWasUnderscore` = false |
   | B3 | `c` is `:` | append `:`; `lastWasUnderscore` = false |
   | B4 | `c` is a Letter | append the UTF-8 encoding of lower(`c`); `lastWasUnderscore` = false |
   | B5 | `out` is empty | skip `c` (append nothing; state unchanged) |
   | B6 | `c` is `.`, `/` or `-` | append `c`; `lastWasUnderscore` = false |
   | B7 | `c` is a Digit | append the UTF-8 encoding of `c`; `lastWasUnderscore` = false |
   | B8 | `lastWasUnderscore` is false | append `_`; `lastWasUnderscore` = true |
   | B9 | otherwise | skip `c` |

**Finish.** When the input is exhausted or a step stopped: if
`lastWasUnderscore` is true, remove the last byte of `out` (which is `_`). The
result is N(t) = `out`.

Notes, all of which follow from the above and are pinned by vectors:

- The class tests in B4 and B7 apply to `c` itself, before it is lowercased.
- B5 comes after B1 to B4, so a tag can only start with a letter or `:`.
  Everything before the first one is dropped, including digits and `_`.
- `_` in the input is handled by B8 like any other character, which is what
  collapses runs of underscores (and of other characters) into one `_`.
- Only one trailing `_` is ever removed, and only if it was the last byte
  appended. A `_` followed by `:` (`a_:b`) or by `.` (`a_.b`) is kept. Unlike
  metric names, `.` never absorbs a `_`.
- Step 1 is checked before appending, so the last code point can take the
  output past `MaxLength`. The output is at most `MaxLength + 3` = 203 bytes.
- Step 2 compares the input offset at which `c` starts, with a strict `>`. A
  code point starting at offset 400 is processed. One starting at 401 is not.
- N(t) may be empty. The intake drops empty tags.

## 5. Tag key: K(t)

The **key** of a tag is the part of the normalized tag before its first `:`:

> K(t) = the prefix of N(t) before its first `:`, or N(t) if it contains no `:`.

This is the key the intake stores, and what tag-key matching compares.

**Streaming form.** K(t) can be computed without normalizing the value. Run the
state machine of §4, but stop as soon as branch B3 is selected: K(t) is `out` at
that point, *without* the Finish step's underscore removal. If B3 is never
selected, K(t) is N(t), Finish step included. The two forms are equal because
`out` is append-only and a `:` resets `lastWasUnderscore`, so nothing after the
first emitted `:` can change the bytes before it.

Consequences, pinned by vectors:

- `a_:b` has key `a_`, but the bare tag `a_` has key `a`.
- `:foo`, `123:foo` and `_:foo` all have the empty key.
- A `:` that would be byte 201 or later is never emitted. A tag whose
  first 200 bytes contain no `:` therefore has no value: its key is its whole
  (truncated) normalized form.

## 6. Rule key: R(k)

A rule, such as a `metric_tag_filterlist` entry, names tag keys. A rule key is
normalized into the key the intake stores for tags with that key:

> R(k) is **rejected** if k contains a `:` byte. Otherwise R(k) = K(k + ":").

Appending `:` makes `k` normalize as a key rather than as a bare tag, so a
trailing `_` that is part of a stored key survives: R(`foo_`) = `foo_`. A rule
key that already is a stored key is unchanged (§8, P4), so rules authored
against what users see in Datadog are unaffected.

R(k) may be empty. The empty key is a stored key: the intake stores `:foo`, and
`123:foo`, as `:foo`. So the rule keys ``, `123` and `_` all match those tags.

A rejected rule key can never match a stored tag. Implementations MUST NOT
match it against anything. They SHOULD report it.

## 7. Fast paths (informative)

Implementations MAY skip the state machine when they can prove the result. Any
fast path MUST produce exactly the result of §4 to §6 (§8, P5). The common
case is a tag whose key is already normalized ASCII:

- Only the bytes up to the first `:` are read. The key is empty if the tag is
  empty or starts with `:`. Otherwise the first byte must be in `a`–`z`, and the
  following bytes in `a`–`z0-9./-`, or `_` not preceded by `_`. If a `:` is then
  found at byte index `i < MaxLength`, K(t) = t[:i]. If there is no `:`, the
  whole tag must be at most `MaxLength` bytes and not end in `_`, and K(t) = t.
  In every other case the fast path does not apply.

## 8. Properties

For every input t (any bytes) and every rule key k, a conforming implementation
satisfies:

- **P1** N(N(t)) = N(t).
- **P2** K(t) is the prefix of N(t) before its first `:`, or N(t) if there is none.
- **P3** K(N(t)) = K(t).
- **P4** R(K(t)) = K(t). Every stored key, including the empty key, is accepted
  and unchanged.
- **P5** Every fast path agrees with the state machine wherever it applies.
- **P6** N(t) and K(t) are valid UTF-8 and at most 203 bytes. N(t) does not end
  in `_`. K(t) contains no `:`. A non-empty N(t) starts with `:` or a Letter.
- **P7** R(k) is rejected whenever k contains `:`.

The Datadog Agent checks P1 to P7 on every vector and with a fuzz target
(`FuzzProperties`).

## 9. Test vectors

`testdata/vectors.txt` holds one vector per line: three strings, the input,
N(input) and K(input). `testdata/rule_vectors.txt` holds an input and either
R(input) or the bare word `reject`. A `#` outside a string starts a comment.

Strings use Go `strconv.QuoteToASCII` syntax. Printable ASCII is literal, and
every other code point is written as `\uNNNN` or `\UNNNNNNNN`, so inputs that
look the same when rendered (such as composed and decomposed `é`) stay distinct.
Bytes that are not valid UTF-8 are written as `\xNN`. The full escape grammar is
that of Go interpreted string literals.

The vectors are normative (§2). Each one exercises a clause of this document,
named in its comment. Expected values come from the intake's own
implementation and are not edited by hand. If the intake's rules change,
regenerate the vectors from it, update this document, and check that the
implementations still pass.

## 10. Use in the tag filter list (informative)

A `metric_tag_filterlist` rule applies to one metric name and lists tag keys.
In the Datadog Agent, rules are compiled in
`comp/filterlist/impl/tagfilterlist.go` and matched in
`pkg/metrics/metricname/tagmatcher.go`.
To agree with the intake:

- The rule's metric name is compared after metric-name normalization
  on both the rule side and the sample side.
- Each listed key is replaced by R(key). Rejected keys are dropped.
- A sample tag t matches the list when K(t) equals one of the rule keys.
- Entries from the same source (local configuration or Remote Configuration)
  that name the same metric after normalization are merged (same action: union
  of keys; conflicting actions: exclude wins). Remote Configuration replaces the
  local configuration rather than merging with it.
- The tags that are kept are still sent verbatim. The intake normalizes them as
  usual.
- Configuration is reported as received. Normalization happens only inside the
  compiled matcher.

## 11. Known gaps and maintenance

- **Arbitrary-tag-value mode.** With `Allow-Arbitrary-Tag-Value: true` the
  intake normalizes series tags with `NormalizeTagArbTagValue`. That function
  normalizes keys differently. It applies NFC, does not allow a leading `:`,
  strips a trailing `_` from the key, and strips trailing whitespace and `:` from
  the tag. This specification does not cover it, so key matching for series sent
  with that header can disagree with the intake on such inputs. The Datadog
  Agent and ADP send the header when `allow_arbitrary_tags` is enabled.
  Sketches are unaffected.
- **Unicode version.** §3.2 and §3.3 follow the Unicode version of each
  implementation's library: Go's `unicode` package for the intake and this
  package, `icu_properties` and the Rust standard library for Agent Data Plane.
  These versions can differ. In practice, implementations then disagree only on
  code points that the older version leaves unassigned, which are neither
  Letters nor Digits there, so only tags whose keys contain recently assigned
  characters can match differently. That difference is accepted.
- **Changes in the intake.** Its tag rules have been stable, but change more
  often than its metric-name rules. See §9 for updating the vectors.
- **Copies.** ADP keeps a copy of this document and of the vector files. The
  copy is identical apart from paragraphs marked **In ADP**. Update both in the
  same change.
