# `github.com/go-logr/logr/funcr` patch

## What is this?

This is an implementation of the `github.com/go-logr/logr/funcr` package from `github.com/go-logr/logr@v1.2.2` which is compatible with v0.4.0. 

It is exposed as a module and declares its path to be `github.com/go-logr/logr/funcr`.

## What is the motivation behind this?

We need this because of a 'dependency hell' situation:

1. `k8s.io/component-base` depends on v0.4.0 of `github.com/go-logr/logr`
2. `go.opentelemetry.io/otel` depends on v1.2.2 of `github.com/go-logr/logr`.
3. Go considers v0.x and v1.x to be the same major version, presumably for backwards compatibility with the era pre-modules.

This situation would be solved by bumping Kubernetes to v0.23.0 or above, see [relevant commit](https://github.com/kubernetes/kubernetes/commit/cb6a6537).
However, Kubernetes can't be upgraded above v0.21.x, because of another 'dependency hell': [it depends on v0.20 of `go.opentelemetry.io/otel`](https://github.com/kubernetes/kubernetes/issues/106536). Preserving backwards-compatibility with older Kubernetes API objects also makes the update difficult.

This module is used to solve this dependency hell, by adding the `github.com/go-logr/logr/funcr` package into version v0.4.0 of the logr dependency.

It is the smallest patch that can be applied to solve this particular 'dependency hell' issue with logr. It will not fix other issues when packages depend on other, newer packages, or when packages depend on functions/structs not present on logr v0.4.0 (e.g. LogSink).

## When can it be removed?

This must be removed when logr is bumped to v1.0.0 or above.

## How does it work?

It bundles a copy of `github.com/go-logr/logr@v1.2.2` on the `internal/logr` folder, and uses a wrapper (see `internal/wrapper`) to make a v1.2.2 logger into a v0.4.0 logger (which, luckily, is an interface). Furthermore:

1. Non-public references to logr are replaced with references to the `internal/logr` copy. 
2. Public references are kept referencing v0.4.0 logr.
3. The `New` and `NewJSON` functions are rewritten using the wrapper.

Code in `internal/logr` is kept as-is on the original dependency. Code on the funcr package has the following diff:

<details>

<summary> Diff of funcr.go </summary>

```diff
--- /Users/pablo.baeyens/Source/logr/funcr/funcr.go     2022-03-04 13:16:46.000000000 +0100
+++ internal/patch/logr/funcr/funcr.go  2022-03-04 13:50:08.000000000 +0100
@@ -46,11 +46,13 @@
        "time"
 
        "github.com/go-logr/logr"
+       v122Logr "github.com/go-logr/logr/funcr/internal/logr"
+       "github.com/go-logr/logr/funcr/internal/wrapper"
 )
 
 // New returns a logr.Logger which is implemented by an arbitrary function.
 func New(fn func(prefix, args string), opts Options) logr.Logger {
-       return logr.New(newSink(fn, NewFormatter(opts)))
+       return wrapper.Fromv122Logger(v122Logr.New(newSink(fn, NewFormatter(opts))))
 }
 
 // NewJSON returns a logr.Logger which is implemented by an arbitrary function
@@ -59,7 +61,7 @@
        fnWrapper := func(_, obj string) {
                fn(obj)
        }
-       return logr.New(newSink(fnWrapper, NewFormatterJSON(opts)))
+       return wrapper.Fromv122Logger(v122Logr.New(newSink(fnWrapper, NewFormatterJSON(opts))))
 }
 
 // Underlier exposes access to the underlying logging function. Since
@@ -70,7 +72,7 @@
        GetUnderlying() func(prefix, args string)
 }
 
-func newSink(fn func(prefix, args string), formatter Formatter) logr.LogSink {
+func newSink(fn func(prefix, args string), formatter Formatter) v122Logr.LogSink {
        l := &fnlogger{
                Formatter: formatter,
                write:     fn,
@@ -157,17 +159,17 @@
        write func(prefix, args string)
 }
 
-func (l fnlogger) WithName(name string) logr.LogSink {
+func (l fnlogger) WithName(name string) v122Logr.LogSink {
        l.Formatter.AddName(name)
        return &l
 }
 
-func (l fnlogger) WithValues(kvList ...interface{}) logr.LogSink {
+func (l fnlogger) WithValues(kvList ...interface{}) v122Logr.LogSink {
        l.Formatter.AddValues(kvList)
        return &l
 }
 
-func (l fnlogger) WithCallDepth(depth int) logr.LogSink {
+func (l fnlogger) WithCallDepth(depth int) v122Logr.LogSink {
        l.Formatter.AddCallDepth(depth)
        return &l
 }
@@ -187,8 +189,8 @@
 }
 
 // Assert conformance to the interfaces.
-var _ logr.LogSink = &fnlogger{}
-var _ logr.CallDepthLogSink = &fnlogger{}
+var _ v122Logr.LogSink = &fnlogger{}
+var _ v122Logr.CallDepthLogSink = &fnlogger{}
 var _ Underlier = &fnlogger{}
 
 // NewFormatter constructs a Formatter which emits a JSON-like key=value format.
@@ -224,7 +226,7 @@
 
 // Formatter is an opaque struct which can be embedded in a LogSink
 // implementation. It should be constructed with NewFormatter. Some of
-// its methods directly implement logr.LogSink.
+// its methods directly implement v122Logr.LogSink.
 type Formatter struct {
        outputFormat outputFormat
        prefix       string
@@ -348,7 +350,7 @@
        }
 
        // Handle types that take full control of logging.
-       if v, ok := value.(logr.Marshaler); ok {
+       if v, ok := value.(v122Logr.Marshaler); ok {
                // Replace the value with what the type wants to get logged.
                // That then gets handled below via reflection.
                value = invokeMarshaler(v)
@@ -597,7 +599,7 @@
        return false
 }
 
-func invokeMarshaler(m logr.Marshaler) (ret interface{}) {
+func invokeMarshaler(m v122Logr.Marshaler) (ret interface{}) {
        defer func() {
                if r := recover(); r != nil {
                        ret = fmt.Sprintf("<panic: %s>", r)
@@ -692,7 +694,7 @@
 // Init configures this Formatter from runtime info, such as the call depth
 // imposed by logr itself.
 // Note that this receiver is a pointer, so depth can be saved.
-func (f *Formatter) Init(info logr.RuntimeInfo) {
+func (f *Formatter) Init(info v122Logr.RuntimeInfo) {
        f.depth += info.CallDepth
 }

```

</details>
