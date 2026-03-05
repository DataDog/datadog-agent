// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func newOverlayEnviron(parent expand.Environ, background bool) *overlayEnviron {
	oenv := &overlayEnviron{}
	if !background {
		oenv.parent = parent
	} else {
		// We could do better here if the parent is also an overlayEnviron;
		// measure with profiles or benchmarks before we choose to do so.
		oenv.values = make(map[string]expand.Variable)
		maps.Insert(oenv.values, parent.Each)
	}
	return oenv
}

// overlayEnviron is our main implementation of [expand.WriteEnviron].
type overlayEnviron struct {
	// parent is non-nil if [values] is an overlay over a parent environment
	// which we can safely reuse without data races, such as non-background subshells.
	parent expand.Environ
	values map[string]expand.Variable
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
	if vr.Kind == expand.KeepValue {
		vr.Kind = prev.Kind
		vr.Str = prev.Str
		vr.List = prev.List
		vr.Map = prev.Map
	} else if prev.ReadOnly {
		return fmt.Errorf("readonly variable")
	}
	if !vr.IsSet() { // unsetting
		if prev.Local {
			vr.Local = true
			o.values[name] = vr
			return nil
		}
		delete(o.values, name)
		return nil
	}
	// modifying the entire variable
	vr.Local = prev.Local || vr.Local
	o.values[name] = vr
	return nil
}

func (o *overlayEnviron) Each(f func(name string, vr expand.Variable) bool) {
	if o.parent != nil {
		o.parent.Each(f)
	}
	for name, vr := range o.values {
		if !f(name, vr) {
			return
		}
	}
}

func (r *Runner) lookupVar(name string) expand.Variable {
	if name == "" {
		panic("variable name must not be empty")
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

func (r *Runner) setVar(name string, vr expand.Variable) {
	if err := r.writeEnv.Set(name, vr); err != nil {
		r.errf("%s: %v\n", name, err)
		r.exit.code = 1
		return
	}
}

// setVarWithIndex sets a variable.  In safe-shell, arrays and indexing are
// blocked by the AST validator, so we only handle simple string assignment.
func (r *Runner) setVarWithIndex(prev expand.Variable, name string, index syntax.ArithmExpr, vr expand.Variable) {
	if index != nil {
		panic("setVarWithIndex: index should have been rejected by AST validation")
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
		panic("assignVal: append should have been rejected by AST validation")
	}
	if as.Array != nil {
		panic("assignVal: array assignment should have been rejected by AST validation")
	}
	if as.Value != nil {
		prev.Kind = expand.String
		prev.Str = r.literal(as.Value)
		return prev
	}
	// Bare assignment (e.g. VAR=)
	prev.Kind = expand.String
	prev.Str = ""
	return prev
}
