# Porting libpcap's grammar.y.in to goyacc

This document describes every modification applied to the original libpcap
1.10.6 `grammar.y.in` to produce the Go-compatible `grammar.y` for goyacc.
A companion script `convert_grammar.sh` automates the mechanical parts.

## Source files

- **Original:** `/path/to/libpcap-1.10.6/grammar.y.in` (949 lines, Bison/YACC C grammar)
- **Ported:** `grammar.y` (goyacc grammar with Go action code)
- **Generated:** `grammar.go` (output of `goyacc -o grammar.go -p yy grammar.y`)

## Modifications by category

### 1. Removed Bison/C-specific directives

The following directives at the top of `grammar.y.in` have no goyacc equivalent
and were removed entirely:

| Directive | Purpose in C | Reason for removal |
|-----------|-------------|-------------------|
| `@REENTRANT_PARSER@` | Autoconf placeholder for `%pure-parser` | goyacc parsers are always reentrant (no global state) |
| `%parse-param {void *yyscanner}` | Pass scanner handle to parser | goyacc uses the `yyLexer` interface instead |
| `%lex-param {void *yyscanner}` | Pass scanner handle to lexer calls | Same — interface-based |
| `%parse-param {compiler_state_t *cstate}` | Pass compiler state to parser | Accessed via `yylex.(*parserLex).cs` in Go |
| `%expect 38` | Suppress shift/reduce conflict warnings | goyacc does not support `%expect`; it reports 38 conflicts on stderr, matching the expected count |
| `DIAG_OFF_BISON_BYACC` / `DIAG_ON_BISON_BYACC` | Suppress compiler warnings in generated C | Not applicable to Go |

### 2. Replaced `%{...%}` prologue (C code → Go imports)

**C version** (lines 47-342): ~300 lines of C includes, helper structs, lookup
tables (`ieee80211_types[]`, `pflog_reasons[]`, `pflog_actions[]`, `llc_s_subtypes[]`,
`llc_u_subtypes[]`), helper functions (`str2tok()`, `yyerror()`, `pfreason_to_num()`,
`pfaction_to_num()`), and macros (`QSET`, `CHECK_INT_VAL`, `CHECK_PTR_VAL`).

**Go version**: Replaced with a minimal `%{ %}` block containing only Go import
statements:

```go
%{
package grammar

import (
    "github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
    "github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

var _ = bpf.BPF_JEQ
%}
```

The C lookup tables and helper functions were either:
- **Moved to Go code** in `scanner.go` (keyword tables) or `codegen/` (protocol constants)
- **Simplified** in grammar actions (e.g., 802.11 type/subtype lookup is stubbed)
- **Replaced by codegen functions** (error-checked calls replace `CHECK_PTR_VAL`/`YYABORT`)

### 3. Translated `%union` (C types → Go types)

| C type | Go type | Field name |
|--------|---------|------------|
| `int i` | `i int` | Same |
| `bpf_u_int32 h` | `h uint32` | Same |
| `char *s` | `s string` | Same |
| `struct stmt *stmt` | *(removed)* | Unused in grammar actions |
| `struct arth *a` | `a *codegen.Arth` | Same |
| `struct { struct qual q; int atmfieldtype; int mtp3fieldtype; struct block *b; } blk` | `blk struct { q codegen.Qual; b *codegen.Block; atmfieldtype int; mtp3fieldtype int }` | Same structure |
| `struct block *rblk` | `rblk *codegen.Block` | Same |

### 4. Combined `%type` and `%token` for typed tokens

Bison allows separate `%type` and `%token` declarations for the same symbol.
goyacc does not — declaring a symbol with both `%type <s> ID` and `%token ID`
causes a "redeclared as nonterminal" error.

**Fix:** Use `%token <type> NAME` to declare both the token and its type:

```
# C (two declarations):
%type  <s>  ID EID AID HID HID6
%type  <h>  NUM
%token ID EID AID HID HID6
%token NUM

# Go (one declaration each):
%token <s>  ID EID HID HID6 AID
%token <h>  NUM
```

### 5. Replaced `$<blk>0` inherited attributes with qualifier stack

This is the most significant structural change. The C grammar uses Bison's
`$<type>0` feature extensively to access the semantic value of the symbol
on the parser stack just *before* the current rule's left-hand side. This
is how the qualifier from `head` flows to `id`/`nid` productions:

```c
// C: inherited attribute via $0
and:  AND   { $$ = $<blk>0; }
nid:  ID    { $$.b = gen_scode(cstate, $1, $$.q = $<blk>0.q); }
```

While some goyacc versions may support `$<type>0`, it is a non-standard
extension. We replaced it with an explicit qualifier stack on `CompilerState`:

```go
// Go: qualifier stack
and:  AND
    {
        cs := yylex.(*parserLex).cs
        $$.q = cs.PeekQual()
    }

nid:  ID
    {
        cs := yylex.(*parserLex).cs
        $$.q = cs.PeekQual()
        $$.b = codegen.GenScode(cs, $1, $$.q)
    }
```

The `head` production pushes the qualifier onto the stack, and `rterm`'s
`head id` production pops it after the id is resolved:

```go
head: pqual dqual aqual
    {
        cs := yylex.(*parserLex).cs
        $$.q = codegen.Qual{Proto: uint8($1), Dir: uint8($2), Addr: uint8($3)}
        cs.PushQual($$.q)
    }

rterm: head id
    {
        cs := yylex.(*parserLex).cs
        cs.PopQual()
        $$ = $2
    }
```

**CompilerState additions** (in `codegen/types.go`):

```go
type CompilerState struct {
    // ...
    qualStack []Qual
}

func (cs *CompilerState) PushQual(q Qual)
func (cs *CompilerState) PopQual()
func (cs *CompilerState) PeekQual() Qual
```

### 6. Replaced C action code with Go action code

Every grammar action was rewritten from C to Go. The key patterns:

| C pattern | Go replacement |
|-----------|---------------|
| `CHECK_PTR_VAL(x)` → `if (x == NULL) YYABORT` | `if x == nil { return 1 }` |
| `CHECK_INT_VAL(x)` → `if (x == -1) YYABORT` | Checked via error return on `cs.Err` |
| `QSET(q, p, d, a)` | `codegen.Qual{Proto: uint8(p), Dir: uint8(d), Addr: uint8(a)}` |
| `bpf_set_error(cstate, "msg"); YYABORT;` | `yylex.Error("msg"); return 1` |
| `qerr` (static const with all `Q_UNDEF`) | `codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}` |
| `cstate` (parser param) | `yylex.(*parserLex).cs` |
| `gen_and($1.b, $3.b)` | `codegen.GenAnd($1.b, $3.b)` |
| `finish_parse(cstate, $2.b)` | `codegen.FinishParse(cs, $2.b)` |
| `BPF_JGT` (C constant) | `bpf.BPF_JGT` (Go package constant) |
| `Q_SRC`, `Q_TCP`, etc. | `codegen.QSrc`, `codegen.QTCP`, etc. |
| `A_LANE`, `M_FISU`, etc. | `codegen.ALane`, `codegen.MFISU`, etc. |

### 7. Accessing compiler state in actions

The C grammar receives `cstate` as a `%parse-param`. In Go, the parser
accesses it through the `yyLexer` interface:

```go
cs := yylex.(*parserLex).cs
```

The `parserLex` struct wraps the `Scanner` and holds a pointer to
`CompilerState`. This pattern appears at the start of every action that
needs the compiler state.

### 8. Simplified 802.11/PF/LLC lookup tables

The C grammar embeds lookup tables for IEEE 802.11 type/subtype names,
PF reasons/actions, and LLC subtypes directly in actions. These were
simplified in the Go port:

- **802.11 `type`/`subtype`/`type_subtype` productions**: Accept `NUM` or
  `ID`, but the `ID` → integer lookup via `str2tok()` is stubbed with `$$ = 0`.
  Full lookup will be added when the IEEE 802.11 codegen is implemented.
- **PF `reason`/`action` productions**: Same — `ID` cases are stubbed.
- **LLC `pllc` production**: The `LLC ID` action dispatches on `"i"`, `"s"`, `"u"`
  directly in Go; subtype lookups via `str2tok(llc_s_subtypes)` / `str2tok(llc_u_subtypes)`
  are stubbed.

### 9. Parser prefix

goyacc uses `-p prefix` to set the function/type prefix. We use:

```
goyacc -o grammar.go -p yy grammar.y
```

This generates `yyParse()`, `yySymType`, etc. with the `yy` prefix.

### 10. `(void) yynerrs` suppression removed

The C `prog` action includes `(void) yynerrs;` to suppress a Clang warning.
This has no Go equivalent and was removed.

## What the conversion script automates

The script `convert_grammar.sh` handles the mechanical transformations:

1. Strip the C prologue (everything from `@REENTRANT_PARSER@` through `%}`)
2. Remove `%parse-param`, `%lex-param`, `%expect`
3. Replace the `%union` block with Go types
4. Merge `%type`/`%token` declarations for typed tokens
5. Replace C constants (`Q_SRC` → `codegen.QSrc`, `BPF_JGT` → `bpf.BPF_JGT`)
6. Replace `$<blk>0` references with `cs.PeekQual()` calls
7. Replace `CHECK_PTR_VAL`/`CHECK_INT_VAL`/`QSET` macros with Go equivalents
8. Replace `cstate` references with `yylex.(*parserLex).cs`

The script produces a *draft* `.y` file that still requires manual review for:
- Complex actions with C control flow (`if`/`else` chains, `for` loops)
- Lookup table references (`str2tok()`)
- Actions that use multiple `$<blk>0` accesses (the push/pop pattern must be correct)

## Validation

The ported grammar produces exactly 38 shift/reduce conflicts, matching the
original `%expect 38` in `grammar.y.in`. This confirms the grammar structure
is identical — only the action code differs.
