// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"errors"
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// MaxVarBytes is the maximum size in bytes of a single variable value.
// Assignments that exceed this limit are rejected with an error.
const MaxVarBytes = 1 << 20 // 1 MiB

// MaxTotalVarsBytes is the maximum total size in bytes of all variable values
// a script holds at once. The budget is shared by every subshell and pipeline
// stage of a run that is alive at the same time. Assignments that would push
// the total over this limit are rejected.
const MaxTotalVarsBytes = 1 << 20 // 1 MiB

// MaxTotalVarEntryBytes is the maximum total of variable names plus a fixed
// per-variable overhead (varEntryOverheadBytes) a script holds at once. It
// bounds the number of variables, which MaxTotalVarsBytes alone does not:
// empty values cost nothing against it. With short names this allows roughly
// 16k variables. It is shared like MaxTotalVarsBytes.
const MaxTotalVarEntryBytes = 1 << 20 // 1 MiB

// varEntryOverheadBytes is charged against MaxTotalVarEntryBytes for every
// stored variable in addition to its name. It approximates the map entry and
// expand.Variable bookkeeping, so that empty-valued variables are not free.
const varEntryOverheadBytes = 64

// varStorage is variable storage charged against the run-wide caps: value
// bytes against MaxTotalVarsBytes, and name bytes plus per-variable overhead
// against MaxTotalVarEntryBytes.
type varStorage struct {
	values  int
	entries int
}

func (a varStorage) add(b varStorage) varStorage {
	return varStorage{values: a.values + b.values, entries: a.entries + b.entries}
}

func (a varStorage) sub(b varStorage) varStorage {
	return varStorage{values: a.values - b.values, entries: a.entries - b.entries}
}

func (a varStorage) grows(from varStorage) bool {
	return a.values > from.values || a.entries > from.entries
}

func (a varStorage) exceedsCaps() bool {
	return a.values > MaxTotalVarsBytes || a.entries > MaxTotalVarEntryBytes
}

// varStorageCost returns what one stored variable is charged. Undeclared
// variables have no entry and cost nothing.
func varStorageCost(name string, vr expand.Variable) varStorage {
	if !vr.Declared() {
		return varStorage{}
	}
	return varStorage{values: len(vr.Str), entries: len(name) + varEntryOverheadBytes}
}

// varStorageBudget is the run-wide variable storage budget shared by every
// overlay derived from the same root environment. live is the sum of the
// storage each live overlay owns (see overlayEnviron.owned). Subshells and
// pipeline stages run on their own overlays, and pipeline stages run
// concurrently, so without a shared budget each one could hold the full caps
// at the same time.
type varStorageBudget struct {
	mu   sync.Mutex
	live varStorage
}

// errTotalVarStorageExceeded is returned by overlayEnviron.Set when the total
// variable storage cap is exceeded. It is a distinct type so that setVar can
// treat it as a fatal error and abort the script.
type errTotalVarStorageExceeded struct {
	total varStorage
}

func (e *errTotalVarStorageExceeded) Error() string {
	if e.total.values > MaxTotalVarsBytes {
		return fmt.Sprintf("variable storage limit exceeded (%d bytes total)", e.total.values)
	}
	return fmt.Sprintf("variable storage limit exceeded (%d bytes of names and entries total)", e.total.entries)
}

func newOverlayEnviron(parent expand.Environ, background bool) *overlayEnviron {
	oenv := &overlayEnviron{}
	if !background {
		oenv.parent = parent
	} else {
		// The copied entries share their storage with the parent, which keeps
		// owning (and being charged for) them while this overlay is alive.
		oenv.values = make(map[string]expand.Variable)
		maps.Insert(oenv.values, parent.Each)
	}
	if pov, ok := parent.(*overlayEnviron); ok {
		// Join the parent's run-wide budget. The parent's storage stays
		// charged there for as long as it is alive, so variables this overlay
		// inherits are counted exactly once, and hiding or replacing them here
		// does not refund storage the parent still holds. Variables provided
		// through Env() are not owned by any overlay and are never counted.
		oenv.budget = pov.sharedBudget()
	} else {
		// Fallback for non-overlayEnviron parents, which have no budget:
		// count every inherited entry.
		var base varStorage
		parent.Each(func(name string, vr expand.Variable) bool {
			base = base.add(varStorageCost(name, vr))
			return true
		})
		oenv.budget = &varStorageBudget{live: base}
	}
	return oenv
}

// overlayEnviron is our main implementation of [expand.WriteEnviron].
type overlayEnviron struct {
	// parent is non-nil if [values] is an overlay over a parent environment
	// which we can safely reuse without data races, such as non-background subshells.
	parent expand.Environ
	values map[string]expand.Variable

	// budget is the run-wide budget shared with every other overlay derived
	// from the same root. It is created lazily for standalone overlays.
	budget *varStorageBudget
	// owned is the storage, per variable, that this overlay itself assigned
	// and holds in budget, counted against MaxTotalVarsBytes and
	// MaxTotalVarEntryBytes (see varStorageCost). Entries inherited from a
	// parent, or copied from it for background subshells, are owned by the
	// parent and absent here. charged is the sum of owned.
	owned   map[string]varStorage
	charged varStorage
}

// sharedBudget returns o's run-wide budget, creating one seeded with o's own
// storage if o has none yet. It is only called while o has no live
// children, so the lazy creation cannot race.
func (o *overlayEnviron) sharedBudget() *varStorageBudget {
	if o.budget == nil {
		o.budget = &varStorageBudget{live: o.charged}
	}
	return o.budget
}

// own records that o now holds vr for name, replacing what o previously owned
// for it, and updates the run-wide budget. When enforce is set and o's
// storage for name grows, the change is rejected if the storage held by every
// live overlay (ancestors, concurrent pipeline stages and their subshells)
// would exceed a cap. The check and the update happen under the budget lock
// so concurrent stages cannot both pass against the same snapshot.
func (o *overlayEnviron) own(name string, vr expand.Variable, enforce bool) error {
	newCost := varStorageCost(name, vr)
	oldCost := o.owned[name]
	delta := newCost.sub(oldCost)
	budget := o.sharedBudget()
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if enforce && newCost.grows(oldCost) {
		if after := budget.live.add(delta); after.exceedsCaps() {
			return &errTotalVarStorageExceeded{total: after}
		}
	}
	budget.live = budget.live.add(delta)
	o.charged = o.charged.add(delta)
	if newCost == (varStorage{}) {
		delete(o.owned, name)
	} else {
		if o.owned == nil {
			o.owned = make(map[string]varStorage)
		}
		o.owned[name] = newCost
	}
	return nil
}

// release returns o's owned storage to the run-wide budget. It must be called
// once the subshell, pipeline stage or temporary overlay using o has
// finished; o must not be written afterwards.
func (o *overlayEnviron) release() {
	if o.budget != nil {
		o.budget.mu.Lock()
		o.budget.live = o.budget.live.sub(o.charged)
		o.budget.mu.Unlock()
	}
	o.owned = nil
	o.charged = varStorage{}
}

// resetAccounting forgets what o owns, so its current variables are not
// counted, and starts a fresh run-wide budget rooted at o.
func (o *overlayEnviron) resetAccounting() {
	o.budget = &varStorageBudget{}
	o.owned = nil
	o.charged = varStorage{}
}

func (o *overlayEnviron) Get(name string) expand.Variable {
	if vr, ok := o.values[name]; ok {
		return vr
	}
	if o.parent != nil {
		return o.parent.Get(name)
	}
	return expand.Variable{}
}

func (o *overlayEnviron) Set(name string, vr expand.Variable) error {
	prev, inOverlay := o.values[name]
	if !inOverlay && o.parent != nil {
		prev = o.parent.Get(name)
	}

	if o.values == nil {
		o.values = make(map[string]expand.Variable)
	}
	if prev.ReadOnly && vr.Kind != expand.KeepValue {
		return errors.New("readonly variable")
	}
	if vr.Kind == expand.KeepValue {
		if prev.ReadOnly {
			return errors.New("readonly variable")
		}
		vr.Kind = prev.Kind
		vr.Str = prev.Str
		vr.List = prev.List
		vr.Map = prev.Map
	}
	if !vr.IsSet() { // unsetting
		// Note: prev.ReadOnly is always false here (guarded by the checks above),
		// but we keep this as defense-in-depth in case future refactors change the flow.
		if prev.ReadOnly {
			return errors.New("readonly variable")
		}
		// Only storage o owns is refunded; a parent's entry stays charged
		// to the parent.
		if prev.Local {
			// The unset local retains its slot but no value.
			vr.Local = true
			_ = o.own(name, vr, false)
			o.values[name] = vr
			return nil
		}
		_ = o.own(name, expand.Variable{}, false)
		delete(o.values, name)
		return nil
	}
	// modifying the entire variable — enforce total storage cap. Only what
	// o previously owned for name is credited: overriding a parent's entry
	// leaves the parent's storage charged, since the parent still holds it.
	vr.Local = prev.Local || vr.Local
	if err := o.own(name, vr, true); err != nil {
		return err
	}
	o.values[name] = vr
	return nil
}

// setUncapped sets a variable without enforcing MaxTotalVarsBytes.  It is
// used by setVarRestore to roll back a variable (e.g. OLDPWD after a failed
// cd) even when the script has filled storage close to the cap.  The owned
// storage is still updated so that subsequent cap checks use an accurate
// baseline.
func (o *overlayEnviron) setUncapped(name string, vr expand.Variable) {
	if o.values == nil {
		o.values = make(map[string]expand.Variable)
	}
	prev, inOverlay := o.values[name]
	if !inOverlay && o.parent != nil {
		prev = o.parent.Get(name)
	}
	vr.Local = prev.Local || vr.Local
	_ = o.own(name, vr, false)
	o.values[name] = vr
}

// bindingSnapshot records o's own binding for a name: its entry, if any, and
// the storage o owns for it.
type bindingSnapshot struct {
	name      string
	vr        expand.Variable
	inOverlay bool
	owned     varStorage
}

// snapshotBinding records o's binding for name so restoreBinding can put it
// back exactly once a temporary assignment (FOO=val cmd) ends.
func (o *overlayEnviron) snapshotBinding(name string) bindingSnapshot {
	vr, inOverlay := o.values[name]
	return bindingSnapshot{name: name, vr: vr, inOverlay: inOverlay, owned: o.owned[name]}
}

// restoreBinding puts back a binding recorded by snapshotBinding, without
// enforcing the storage caps. A name o had no entry for is deleted rather
// than set, so an inherited value shows through again without o taking
// ownership of a copy, and a previously absent name leaves no entry behind.
// o's ownership returns to what it was, whatever the command assigned.
func (o *overlayEnviron) restoreBinding(snap bindingSnapshot) {
	budget := o.sharedBudget()
	budget.mu.Lock()
	delta := snap.owned.sub(o.owned[snap.name])
	budget.live = budget.live.add(delta)
	budget.mu.Unlock()
	o.charged = o.charged.add(delta)
	if snap.owned == (varStorage{}) {
		delete(o.owned, snap.name)
	} else {
		if o.owned == nil {
			o.owned = make(map[string]varStorage)
		}
		o.owned[snap.name] = snap.owned
	}
	if snap.inOverlay {
		if o.values == nil {
			o.values = make(map[string]expand.Variable)
		}
		o.values[snap.name] = snap.vr
	} else {
		delete(o.values, snap.name)
	}
}

func (o *overlayEnviron) Each(f func(name string, vr expand.Variable) bool) {
	// Emit our own overrides first so they take precedence.
	for name, vr := range o.values {
		if !f(name, vr) {
			return
		}
	}
	// Then emit parent variables, skipping any that we already overrode above.
	// Without this guard, a variable that exists in both the parent environment
	// and our overlay (because the script re-assigned it) would be emitted twice,
	// and anything summing Each() would count it twice.
	if o.parent != nil {
		o.parent.Each(func(name string, vr expand.Variable) bool {
			if _, override := o.values[name]; override {
				return true // already emitted from our overlay
			}
			return f(name, vr)
		})
	}
}

// internalErrorf records an internal assertion failure as a fatal error.
// Use this for conditions that should be unreachable (e.g. invariants
// enforced by AST validation).
func (r *Runner) internalErrorf(format string, a ...any) {
	r.exit.fatal(fmt.Errorf("internal error: "+format, a...))
}

func (r *Runner) lookupVar(name string) expand.Variable {
	if name == "" {
		r.internalErrorf("variable name must not be empty")
		return expand.Variable{}
	}
	// Only $? is supported as a special variable in safe-shell.
	if name == "?" {
		return expand.Variable{
			Set:  true,
			Kind: expand.String,
			Str:  strconv.Itoa(int(r.lastExit.code)),
		}
	}
	if vr := r.writeEnv.Get(name); vr.Declared() {
		return vr
	}
	if runtime.GOOS == "windows" {
		upper := strings.ToUpper(name)
		if vr := r.writeEnv.Get(upper); vr.Declared() {
			return vr
		}
	}
	return expand.Variable{}
}

func (r *Runner) setVarString(name, value string) {
	r.setVar(name, expand.Variable{Set: true, Kind: expand.String, Str: value})
}

// setVarErr is like setVar but returns the error instead of recording it as a
// side-effect. Use this when the caller needs to propagate the error (e.g. in
// the expand package's WriteEnviron.Set callback).
func (r *Runner) setVarErr(name string, vr expand.Variable) error {
	return r.writeEnv.Set(name, vr)
}

func (r *Runner) setVar(name string, vr expand.Variable) {
	if vr.IsSet() && len(vr.Str) > MaxVarBytes {
		r.errf("%s: value too large (limit %d bytes)\n", name, MaxVarBytes)
		r.exit.code = 1
		return
	}
	if err := r.writeEnv.Set(name, vr); err != nil {
		r.errf("%s: %v\n", name, err)
		var storageErr *errTotalVarStorageExceeded
		if errors.As(err, &storageErr) {
			// Total storage exhaustion aborts the script (sets exiting without
			// recording a fatal error, so Run returns ExitStatus(1) rather than
			// the raw error, which is consistent with how the test helpers work).
			r.exit.code = 1
			r.exit.exiting = true
		} else {
			r.exit.code = 1
		}
		return
	}
}

// setVarRestore writes a variable back without enforcing any size limits.
// Used to restore inline command variables (e.g. FOO=val cmd) to their
// original values after the command returns, so that inherited variables
// larger than MaxVarBytes or near the total cap can be restored correctly.
// If the underlying env is an overlayEnviron, it bypasses the total-bytes
// cap to avoid erroneously blocking the restore when the script filled storage
// during the command's execution.
func (r *Runner) setVarRestore(name string, vr expand.Variable) {
	if ov, ok := r.writeEnv.(*overlayEnviron); ok {
		ov.setUncapped(name, vr)
		return
	}
	if err := r.writeEnv.Set(name, vr); err != nil {
		r.errf("%s: %v\n", name, err)
		r.exit.code = 1
	}
}

// setVarWithIndex sets a variable.  In safe-shell, arrays and indexing are
// blocked by the AST validator, so we only handle simple string assignment.
func (r *Runner) setVarWithIndex(prev expand.Variable, name string, index syntax.ArithmExpr, vr expand.Variable) {
	if index != nil {
		r.internalErrorf("setVarWithIndex: index should have been rejected by AST validation")
		return
	}
	prev.Set = true
	if name2, var2 := prev.Resolve(r.writeEnv); name2 != "" {
		name = name2
		prev = var2
	}
	r.setVar(name, vr)
}

// assignVal evaluates the value of an assignment.  In safe-shell, only simple
// string assignments are supported (no append, no arrays, no NameRef).  The AST
// validator rejects those constructs before we get here, so hitting them is a
// programming error.
func (r *Runner) assignVal(prev expand.Variable, as *syntax.Assign, _ string) expand.Variable {
	prev.Set = true
	if as.Append {
		r.internalErrorf("assignVal: append should have been rejected by AST validation")
		return expand.Variable{}
	}
	if as.Array != nil {
		r.internalErrorf("assignVal: array assignment should have been rejected by AST validation")
		return expand.Variable{}
	}
	if as.Value != nil {
		prev.Kind = expand.String
		prev.Str = r.assignmentLiteral(as.Name.Value, as.Value)
		return prev
	}
	// Bare assignment (e.g. VAR=)
	prev.Kind = expand.String
	prev.Str = ""
	return prev
}
