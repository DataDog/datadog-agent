# Go Generics: Shapes, Dictionaries, and DWARF

This document describes how the Go compiler implements generics at the binary
level, and what that means for dynamic instrumentation. Understanding this is
essential for probing generic functions and correctly interpreting their
parameters.

## Shape Stenciling

The Go compiler does not fully monomorphize generic functions. Instead, it uses
**GC shape stenciling**: type arguments that have the same garbage collection
shape share a single compiled function body. The function operates on the shape
type and uses a runtime **dictionary** to recover concrete type information when
needed (e.g., for method calls, type assertions, reflection).

For a generic function:

```go
func Contains[T comparable](haystack []T, needle T) bool { ... }
```

Called with `int` and `string`, the compiler emits:

| Symbol | What it is |
|--------|-----------|
| `pkg.Contains[go.shape.int]` | Shape function for int-shaped types |
| `pkg.Contains[go.shape.string]` | Shape function for string-shaped types |
| `pkg.Contains[int]` | Trampoline: tail-calls shape function with `&.dict.Contains[int]` |
| `pkg.Contains[string]` | Trampoline: tail-calls shape function with `&.dict.Contains[string]` |
| `pkg..dict.Contains[int]` | Dictionary for the `int` instantiation |
| `pkg..dict.Contains[string]` | Dictionary for the `string` instantiation |

The shape functions contain the real code. The trampolines are thin wrappers
marked with `DW_AT_trampoline` in DWARF.

## What Determines a Shape

Two rules control which types share a shape:

1. **Underlying type identity.** Types with the same `Underlying()` share a
   shape. So `int` and `type MyInt int` both use `go.shape.int`. But `int` and
   `int64` do NOT share a shape — they have different underlying types, even on
   platforms where they're the same size.

2. **Pointer collapsing for basic interfaces.** When a type parameter is
   constrained by a basic interface (one with no methods — like `any` or
   `comparable`), all pointer type arguments collapse to `go.shape.*uint8`.
   So `*Foo`, `*Bar`, and `*string` all share the same shape function. This
   is sound because basic interfaces don't permit accessing the pointee's
   structure. When the constraint has methods, pointers keep their distinct
   shapes.

Consequence: a single shape function can serve **multiple concrete types**. The
dictionary is the runtime discriminator — it tells the function which concrete
type it's operating on.

## The Dictionary

The dictionary is a flat read-only array of `uintptr`-sized words. It has four
sections laid out sequentially:

```
Offset (words)          Section
──────────────          ─────────────────────────────────
0                       typeParamMethodExprs
                        Function pointers for methods called on type parameters.
                        When generic code calls a method on T, it reads the
                        concrete method address from here.

+len(methodExprs)       subdicts
                        Pointers to sub-dictionaries for nested generic calls.
                        If F[T] calls G[T], F's dictionary has a pointer to
                        G's dictionary instantiated with the same types.

+len(subdicts)          rtypes
                        *runtime._type pointers for derived types.
                        Used for reflection, type assertions, and conversions.

+len(rtypes)            itabs
                        *runtime.itab pointers for converting type parameters
                        to non-empty interfaces.
```

Dictionary symbols are named `pkg..dict.Func[ConcreteType]` and placed in
RODATA with DUPOK (linker deduplication).

### Dictionary parameter location

For **out-of-line shape functions**, the dictionary is passed as a hidden
parameter:

- **Generic functions:** the dictionary is the first regular parameter
  (before all user-visible parameters), in int register 0.
- **Methods on generic types:** the dictionary comes after the receiver.
  If the receiver fits in registers, the dictionary gets the next available
  int register. If the receiver is too large for register assignment (e.g.,
  contains arrays of length > 1), the Go ABI passes it entirely on the stack,
  consuming zero registers, and the dictionary gets int register 0 — the same
  position as a free function. Per the Go ABI spec
  (`src/cmd/compile/abi-internal.md`, assignment algorithm step 4): when
  register assignment fails, I and FP are reset and the value is placed on the
  stack. There is no hidden-pointer indirection for oversized parameters.

The parameter is named `.dict` and typed as `*[N]uintptr`. It follows the
standard Go register-based calling convention (ABIInternal) — there is no
special register reserved for it.

For **closures inside generic functions**, the dictionary is NOT a parameter.
Instead, it is captured as the **last variable** in the closure struct. The
closure shares its parent's dictionary — no separate dictionary is generated
for it.

### Dictionary parameter in DWARF

As of Go 1.23–1.26, the `.dict` parameter is **not emitted to DWARF**. The
compiler adds it at the IR level but strips it from debug info before DWARF
generation. Delve (the Go debugger) knows about this and infers the
dictionary's location from its fixed position in the parameter list.

The name `.dict` is a convention shared between the compiler and Delve:
https://github.com/go-delve/delve/blob/cb91509630529e6055be845688fd21eb89ae8714/pkg/proc/eval.go#L28

## DWARF Representation

### Shape function entries

Shape functions are normal `DW_TAG_subprogram` entries (not trampolines).
Their children include:

- **`DW_TAG_typedef` entries** named `.param0`, `.param1`, etc. Each typedef
  has:
  - `DW_AT_type`: reference to the shape type (e.g., `go.shape.int`)
  - `DW_AT_go_dict_index` (0x2906): the index into the dictionary array
    where the concrete `*runtime._type` for this type parameter lives

- **`DW_TAG_formal_parameter` entries** for the user-visible parameters.
  Their types reference the typedef names (`.param0`, `.param1`), not the
  shape types directly.

The `.dict` parameter itself does **not** appear as a formal parameter in DWARF.

### Trampoline entries

Concrete-type wrappers (e.g., `pkg.Contains[int]`) are marked with
`DW_AT_trampoline` and have the concrete parameter types. They are typically
skipped during instrumentation since they just tail-call the shape function.

### Example

For `main.genericContains[go.shape.int]`:

```
DW_TAG_subprogram "main.genericContains[go.shape.int]"
  DW_TAG_typedef ".param0"
    DW_AT_type → []go.shape.int
    DW_AT_go_dict_index → 0
  DW_TAG_typedef ".param1"
    DW_AT_type → go.shape.int
    DW_AT_go_dict_index → 1
  DW_TAG_formal_parameter "haystack"
    DW_AT_type → .param0
  DW_TAG_formal_parameter "needle"
    DW_AT_type → .param1
  DW_TAG_formal_parameter "~r0"
    DW_AT_type → bool
```

## Compile Unit Placement

Shape functions and trampolines appear in the DWARF compile unit of the
**instantiating package**, not the defining package. When `main` calls
`lib.Filter[int]`, the shape function `lib.Filter[go.shape.int]` and the
trampoline `lib.Filter[int]` both end up in `main`'s compile unit — even
though they logically belong to `lib`.

### Why this happens

The Go compiler processes generics during the compilation of the instantiating
package. When `main` is compiled and references `lib.Filter[int]`:

1. The compiler reads `lib.Filter`'s generic definition from `lib`'s export
   data (`cmd/compile/internal/noder/reader.go`, `readBodies`).
2. It creates the shape function `lib.Filter[go.shape.int]` locally and adds
   it to `main`'s `target.Funcs` (`unified.go`, line ~277).
3. It creates the non-shaped trampoline `lib.Filter[int]` as a wrapper that
   tail-calls the shape function (`reader.go`, `syntheticBody`/`callShaped`).
4. Both functions are emitted into `main`'s object file with DWARF entries.

Both the shape function and trampoline are marked `DupOk` (duplicate-OK),
meaning each package that instantiates them compiles its own copy. The linker
deduplicates: the first library to claim a DUPOK symbol wins
(`cmd/link/internal/loader/loader.go`, DUPOK resolution). The winning
definition's compile unit owns the DWARF entry.

### Implications

- **A shape function's DWARF entry may be in any compile unit** — whichever
  package first instantiated that shape. There is no guarantee it appears in
  the defining package's compile unit.
- **A shape function appears in exactly one compile unit** in the final binary,
  despite being compiled by multiple packages. The linker deduplicates.
- **Compile unit ordering is deterministic** for a given build but not
  specified. The linker processes libraries in dependency order: `main` is
  added first to `ctxt.Library` (`cmd/link/internal/ld/main.go`, line ~362),
  then dependencies are loaded during `loadlib`. Internal packages (runtime)
  are processed first, then external packages.
- **Symbol database extraction must account for displacement.** When iterating
  DWARF compile units, functions found in a CU may belong to a different
  package. The function's qualified name (which includes the defining package
  path) is the authoritative source of package membership, not the enclosing
  compile unit.

## Implications for Dynamic Instrumentation

### Probing

Shape functions are the correct probe targets — they contain the real code.
Trampolines should be skipped (they're already filtered by the
`DW_AT_trampoline` check).

A single probe on a shape function fires for ALL concrete types that share
that shape. For example, probing `genericContains[go.shape.int]` fires for
calls with `int`, `myInt`, or any other type whose underlying type is `int`.

### Type resolution at probe time

When a probe fires on a shape function, the captured parameter types are shape
types (e.g., `go.shape.int`), not the caller's concrete types. To resolve the
actual concrete type:

1. Read the dictionary pointer from its ABI register (see "Dictionary
   parameter location" above).
2. Index into the dictionary using the `DW_AT_go_dict_index` from the
   variable's typedef. Note: each variable (parameter or return value)
   references a specific typedef, and different typedefs may have different
   dict indices. For example, `func F[T](p *T) T` has `.param0` for `*T`
   (with one dict index) and `.param1` for `T` (with a different dict index).
3. The dictionary entry is a `*runtime._type` pointer for the concrete type.
4. Look up the concrete type using the existing `typesByGoRuntimeType` mapping.

This is architecturally identical to interface type resolution, where the
runtime type is read from the interface value's type word.

For closures inside generic functions, the dictionary is in the closure struct
(last captured variable) rather than a parameter, requiring an additional
pointer dereference.

### Limitations

- The `.dict` parameter is absent from DWARF, so its location must be inferred
  from ABI knowledge (first param position).
- The dictionary layout (number of entries per section) is not encoded in DWARF.
  The `DW_AT_go_dict_index` values are flat indices into the full array, so
  they can be used directly without knowing section boundaries.
- Multiple concrete types can map to the same shape. Without reading the
  dictionary at runtime, it's impossible to distinguish between them statically.

## References

- [GC Shape Stenciling design doc](https://github.com/golang/proposal/blob/master/design/generics-implementation-gcshape.md)
- [Dictionaries design doc](https://github.com/golang/proposal/blob/master/design/generics-implementation-dictionaries.md)
- [Go 1.18 implementation details](https://github.com/golang/proposal/blob/master/design/generics-implementation-dictionaries-go1.18.md)
- Compiler source: `cmd/compile/internal/noder/reader.go` — `shapeSig`,
  `shapify`, `dictNameOf`, `funcLit`
- DWARF constant: `DW_AT_go_dict_index = 0x2906`
  (`cmd/compile/internal/irgen/dwarf_constants.go` in this repo)
