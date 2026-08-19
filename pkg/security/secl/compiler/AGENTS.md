# SECL compiler

The SECL compiler turns a rule expression into a closure. It has two stages,
over three packages:

```
source ──parse──▶ AST ──compile──▶ evaluators
         parser   ast   eval
```

- `parser` — a hand written recursive descent parser, one function per
  precedence level (`parser.go`), pulling tokens from a scanner (`scanner.go`).
  It reports every syntax error it finds, with a position, a message and an
  optional hint (`errors.go`). `ParsingContext.BuildRule`, `BuildMacro` and
  `BuildExpression` are the entry points of the compiler; the context caches the
  trees of the rules.
- `ast` — the AST. One type per construct of the language, all members of the
  closed `ast.Node` union, and the `ast.Position` of the nodes. No dependency on
  the parser, no dependency on the evaluators.
- `eval` — the evaluators, and their construction from the AST (`compile.go`).
  `eval.Compile` walks the AST, type checks it against a `Model`, and returns a
  tree of closures. `eval.Rule` and `eval.Macro` hold the AST of an expression
  and compile it on `GenEvaluator`.

## Conventions

- **Adding a construct to the language** means touching the three packages: a
  token in `token.go` and `scanner.go` if it needs one, a parsing function at the
  right precedence level in `parser.go`, a node type in `ast`, and a case in
  `compile.go`.
- **The parser looks one token ahead and never backtracks.** A new construct
  should be recognizable from its first token, or from the token following an
  operand for a postfix one (a call, an index).
- **Nodes carry their source position** (`ast.Meta`), and the position of a node
  is the position of its first token. Compilation errors are reported at those
  positions, and evaluators embed the offset of the literal they come from —
  a wrong position silently corrupts error messages and rule filters.
- **Don't resolve names in the parser.** Whether an identifier is a field, a
  constant, a macro or an iterator depends on the `Model` and the `Opts`, which
  are only known at compile time. The AST keeps identifiers as written.
- **Treat the AST as immutable.** Once the parser returns a tree, nothing
  modifies it: a tree is cached per expression, shared by the rules using it,
  and compiled more than once, once per rule and once per partial. A
  transformation returns a new tree, sharing the nodes it leaves unchanged.
- **Beware of shared evaluators.** The value of a macro or of a constant is a
  single evaluator instance reused by every rule referring to it. Compilation
  mutates its operands in a couple of places (`=~` forcing the value type of its
  right operand, `Unary` flagging its operand as deterministic), which leaks
  across rules; copy rather than add to those.

## Testing

`parser/parser_test.go` asserts the shape of the tree with `ast.Sprint`, which
renders it as an s-expression, and the rendering of syntax errors with
`ErrorList.Format`. Prefer them to hand-written structural assertions.
