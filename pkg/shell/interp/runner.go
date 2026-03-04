// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func (r *Runner) fillExpandConfig(ctx context.Context) {
	r.ectx = ctx
	r.ecfg = &expand.Config{
		Env: expandEnv{r},
		// CmdSubst is intentionally nil: command substitution is blocked
		// at the AST validation level, and a nil handler causes the expand
		// package to return UnexpectedCommandError as defense in depth.
	}
	r.updateExpandOpts()
}


func (r *Runner) updateExpandOpts() {
	r.ecfg.ReadDir2 = func(s string) ([]fs.DirEntry, error) {
		return r.readDirHandler(r.handlerCtx(r.ectx, todoPos), s)
	}
}

func (r *Runner) expandErr(err error) {
	if err == nil {
		return
	}
	errMsg := err.Error()
	fmt.Fprintln(r.stderr, errMsg)
	switch {
	case errors.As(err, &expand.UnsetParameterError{}):
	case errMsg == "invalid indirect expansion":
		// TODO: These errors are treated as fatal by bash.
		// Make the error type reflect that.
	case strings.HasSuffix(errMsg, "not supported"):
		// TODO: This "has suffix" is a temporary measure until the expand
		// package supports all syntax nodes like extended globbing.
	default:
		return // other cases do not exit
	}
	r.exit.code = 1
	r.exit.exiting = true
}


func (r *Runner) fields(words ...*syntax.Word) []string {
	strs, err := expand.Fields(r.ecfg, words...)
	r.expandErr(err)
	return strs
}

func (r *Runner) literal(word *syntax.Word) string {
	str, err := expand.Literal(r.ecfg, word)
	r.expandErr(err)
	return str
}

func (r *Runner) document(word *syntax.Word) string {
	str, err := expand.Document(r.ecfg, word)
	r.expandErr(err)
	return str
}

// expandEnviron exposes [Runner]'s variables to the expand package.
type expandEnv struct {
	r *Runner
}

var _ expand.WriteEnviron = expandEnv{}

func (e expandEnv) Get(name string) expand.Variable {
	return e.r.lookupVar(name)
}

func (e expandEnv) Set(name string, vr expand.Variable) error {
	e.r.setVar(name, vr)
	return nil // TODO: return any errors
}

func (e expandEnv) Each(fn func(name string, vr expand.Variable) bool) {
	e.r.writeEnv.Each(fn)
}

var todoPos syntax.Pos // for handlerCtx callers where we don't yet have a position

func (r *Runner) handlerCtx(ctx context.Context, pos syntax.Pos) context.Context {
	hc := HandlerContext{
		Env:    &overlayEnviron{parent: r.writeEnv},
		Dir:    r.Dir,
		Pos:    pos,
		Stdout: r.stdout,
		Stderr: r.stderr,
	}
	if r.stdin != nil { // do not leave hc.Stdin as a typed nil
		hc.Stdin = r.stdin
	}
	return context.WithValue(ctx, handlerCtxKey{}, hc)
}

func (r *Runner) out(s string) {
	io.WriteString(r.stdout, s)
}

func (r *Runner) outf(format string, a ...any) {
	fmt.Fprintf(r.stdout, format, a...)
}

func (r *Runner) errf(format string, a ...any) {
	fmt.Fprintf(r.stderr, format, a...)
}

func (r *Runner) stop(ctx context.Context) bool {
	if r.exit.exiting {
		return true
	}
	if err := ctx.Err(); err != nil {
		r.exit.fatal(err)
		return true
	}
	return false
}

func (r *Runner) stmt(ctx context.Context, st *syntax.Stmt) {
	if r.stop(ctx) {
		return
	}
	r.exit = exitStatus{}
	if st.Background {
		r2 := r.subshell(true)
		st2 := *st
		st2.Background = false
		bg := bgProc{
			done: make(chan struct{}),
			exit: new(exitStatus),
		}
		r.bgProcs = append(r.bgProcs, bg)
		go func() {
			r2.Run(ctx, &st2)
			r2.exit.exiting = false // subshells don't exit the parent shell
			*bg.exit = r2.exit
			close(bg.done)
		}()
	} else {
		r.stmtSync(ctx, st)
	}
	r.lastExit = r.exit
}

func (r *Runner) stmtSync(ctx context.Context, st *syntax.Stmt) {
	oldIn, oldOut, oldErr := r.stdin, r.stdout, r.stderr
	for _, rd := range st.Redirs {
		cls, err := r.redir(ctx, rd)
		if err != nil {
			r.exit.code = 1
			break
		}
		if cls != nil {
			defer cls.Close()
		}
	}
	if r.exit.ok() && st.Cmd != nil {
		r.cmd(ctx, st.Cmd)
	}
	if st.Negated {
		wasOk := r.exit.ok()
		r.exit = exitStatus{}
		r.exit.oneIf(wasOk)
	}
	r.stdin, r.stdout, r.stderr = oldIn, oldOut, oldErr
}

func (r *Runner) cmd(ctx context.Context, cm syntax.Command) {
	if r.stop(ctx) {
		return
	}

	switch cm := cm.(type) {
	case *syntax.Block:
		r.stmts(ctx, cm.Stmts)
	case *syntax.CallExpr:
		args := cm.Args
		r.lastExpandExit = exitStatus{}
		fields := r.fields(args...)
		if len(fields) == 0 {
			for _, as := range cm.Assigns {
				prev := r.lookupVar(as.Name.Value)
				prev.Local = false

				vr := r.assignVal(prev, as, "")
				r.setVarWithIndex(prev, as.Name.Value, as.Index, vr)
			}
			// If interpreting the last expansion like $(foo) failed,
			// and the expansion and assignments otherwise succeeded,
			// we need to surface that last exit code.
			if r.exit.ok() {
				r.exit = r.lastExpandExit
			}
			break
		}

		type restoreVar struct {
			name string
			vr   expand.Variable
		}
		var restores []restoreVar

		for _, as := range cm.Assigns {
			name := as.Name.Value
			prev := r.lookupVar(name)

			vr := r.assignVal(prev, as, "")
			// Inline command vars are always exported.
			vr.Exported = true

			restores = append(restores, restoreVar{name, prev})

			r.setVar(name, vr)
		}

		r.call(ctx, cm.Args[0].Pos(), fields)
		for _, restore := range restores {
			r.setVar(restore.name, restore.vr)
		}
	case *syntax.BinaryCmd:
		switch cm.Op {
		case syntax.AndStmt, syntax.OrStmt:
			r.stmt(ctx, cm.X)
			if r.exit.ok() == (cm.Op == syntax.AndStmt) {
				r.stmt(ctx, cm.Y)
			}
		case syntax.Pipe, syntax.PipeAll:
			pr, pw, err := os.Pipe()
			if err != nil {
				r.exit.fatal(err) // not being able to create a pipe is rare but critical
				return
			}
			r2 := r.subshell(true)
			r2.stdout = pw
			if cm.Op == syntax.PipeAll {
				r2.stderr = pw
			} else {
				r2.stderr = r.stderr
			}
			r.stdin = pr
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				r2.stmt(ctx, cm.X)
				r2.exit.exiting = false // subshells don't exit the parent shell
				pw.Close()
				wg.Done()
			}()
			r.stmt(ctx, cm.Y)
			pr.Close()
			wg.Wait()
			if r2.exit.fatalExit {
				r.exit.fatal(r2.exit.err) // surface fatal errors immediately
			}
		}
	case *syntax.ForClause:
		switch y := cm.Loop.(type) {
		case *syntax.WordIter:
			name := y.Name.Value
			items := r.Params // for i; do ...

			inToken := y.InPos.IsValid()
			if inToken {
				items = r.fields(y.Items...) // for i in ...; do ...
			}

			for _, field := range items {
				r.setVarString(name, field)
				if r.loopStmtsBroken(ctx, cm.Do) {
					break
				}
			}
		default:
			r.exit.fatal(fmt.Errorf("unsupported loop type: %T", cm.Loop))
		}
	default:
		r.exit.fatal(fmt.Errorf("unsupported command node: %T", cm))
	}
}

func (r *Runner) stmts(ctx context.Context, stmts []*syntax.Stmt) {
	for _, stmt := range stmts {
		r.stmt(ctx, stmt)
	}
}

func (r *Runner) hdocReader(rd *syntax.Redirect) (*os.File, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	// We write to the pipe in a new goroutine,
	// as pipe writes may block once the buffer gets full.
	// We still construct and buffer the entire heredoc first,
	// as doing it concurrently would lead to different semantics and be racy.
	if rd.Op != syntax.DashHdoc {
		hdoc := r.document(rd.Hdoc)
		go func() {
			pw.WriteString(hdoc)
			pw.Close()
		}()
		return pr, nil
	}
	var buf bytes.Buffer
	var cur []syntax.WordPart
	flushLine := func() {
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(r.document(&syntax.Word{Parts: cur}))
		cur = cur[:0]
	}
	for _, wp := range rd.Hdoc.Parts {
		lit, ok := wp.(*syntax.Lit)
		if !ok {
			cur = append(cur, wp)
			continue
		}
		for i, part := range strings.Split(lit.Value, "\n") {
			if i > 0 {
				flushLine()
				cur = cur[:0]
			}
			part = strings.TrimLeft(part, "\t")
			cur = append(cur, &syntax.Lit{Value: part})
		}
	}
	flushLine()
	go func() {
		pw.Write(buf.Bytes())
		pw.Close()
	}()
	return pr, nil
}

func (r *Runner) redir(ctx context.Context, rd *syntax.Redirect) (io.Closer, error) {
	if rd.Hdoc != nil {
		pr, err := r.hdocReader(rd)
		if err != nil {
			return nil, err
		}
		r.stdin = pr
		return pr, nil
	}

	orig := &r.stdout
	if rd.N != nil {
		switch rd.N.Value {
		case "0":
			// Note that the input redirects below always use stdin (0)
			// because we don't support anything else right now.
		case "1":
			// The default for the output redirects below.
		case "2":
			orig = &r.stderr
		default:
			return nil, fmt.Errorf("unsupported redirect fd: %v", rd.N.Value)
		}
	}
	arg := r.literal(rd.Word)
	switch rd.Op {
	case syntax.WordHdoc:
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		r.stdin = pr
		// We write to the pipe in a new goroutine,
		// as pipe writes may block once the buffer gets full.
		go func() {
			pw.WriteString(arg)
			pw.WriteString("\n")
			pw.Close()
		}()
		return pr, nil
	case syntax.DplOut:
		switch arg {
		case "1":
			*orig = r.stdout
		case "2":
			*orig = r.stderr
		case "-":
			*orig = io.Discard // closing the output writer
		default:
			return nil, fmt.Errorf("unhandled %v arg: %q", rd.Op, arg)
		}
		return nil, nil
	case syntax.RdrIn:
		// done further below
	case syntax.DplIn:
		switch arg {
		case "-":
			r.stdin = nil // closing the input file
		default:
			return nil, fmt.Errorf("unhandled %v arg: %q", rd.Op, arg)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unhandled redirect op: %v", rd.Op)
	}
	f, err := r.open(ctx, arg, os.O_RDONLY, 0, true)
	if err != nil {
		return nil, err
	}
	stdin, err := stdinFile(f)
	if err != nil {
		return nil, err
	}
	r.stdin = stdin
	return f, nil
}

func (r *Runner) loopStmtsBroken(ctx context.Context, stmts []*syntax.Stmt) bool {
	oldInLoop := r.inLoop
	r.inLoop = true
	defer func() { r.inLoop = oldInLoop }()
	for _, stmt := range stmts {
		r.stmt(ctx, stmt)
		if r.contnEnclosing > 0 {
			r.contnEnclosing--
			return r.contnEnclosing > 0
		}
		if r.breakEnclosing > 0 {
			r.breakEnclosing--
			return true
		}
	}
	return false
}

func (r *Runner) call(ctx context.Context, pos syntax.Pos, args []string) {
	if r.stop(ctx) {
		return
	}
	name := args[0]
	if IsBuiltin(name) {
		r.exit = r.builtin(ctx, pos, name, args[1:])
		return
	}
	r.exec(ctx, pos, args)
}

func (r *Runner) exec(ctx context.Context, pos syntax.Pos, args []string) {
	r.exit.fromHandlerError(r.execHandler(r.handlerCtx(ctx, pos), args))
}

func (r *Runner) open(ctx context.Context, path string, flags int, mode os.FileMode, print bool) (io.ReadWriteCloser, error) {
	f, err := r.openHandler(r.handlerCtx(ctx, todoPos), path, flags, mode)
	// TODO: support wrapped PathError returned from openHandler.
	switch err.(type) {
	case nil:
		return f, nil
	case *os.PathError:
		if print {
			r.errf("%v\n", err)
		}
	default: // handler's custom fatal error
		r.exit.fatal(err)
	}
	return nil, err
}

