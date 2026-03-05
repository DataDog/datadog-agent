# function-p.tst: test of functions for any POSIX-compliant shell

posix="true"

test_OE -e 0 'effect of function definition (without executing the function)'
func(){ echo foo; } <_no_such_file_
__IN__

test_oE 'grouping as function body'
func(){ echo foo; }
func
__IN__
foo
__OUT__

test_oE 'subshell as function body'
func()(echo foo; exit; echo not reached)
func
:
__IN__
foo
__OUT__

test_oE 'for loop as function body'
func()for i in 1; do echo $i; done
func
__IN__
1
__OUT__

test_oE 'case statement as function body'
func()case 1 in 1) echo foo; esac
func
__IN__
foo
__OUT__

test_oE 'if statement as function body'
func()if echo foo; then echo bar; fi
func
__IN__
foo
bar
__OUT__

test_oE 'while loop as function body'
func()while true; do echo foo; break; done
func
__IN__
foo
__OUT__

test_oE 'until loop as function body'
func()until false; do echo foo; break; done
func
__IN__
foo
__OUT__

test_oE 're-defining a function'
func() { echo foo; }
func() { echo bar; }
func
__IN__
bar
__OUT__

test_oE 'characters in portable name'
_abcXYZ1() { echo foo; }
_abcXYZ1
__IN__
foo
__OUT__

test_oE 'functions and variables belong to separate namespaces'
foo=variable
foo() { echo function; }
echo $foo
foo=X
foo
__IN__
variable
function
__OUT__

test_OE 'redirections apply to function body'
func() { echo foo; cat; } >/dev/null <<END
bar
END
func
__IN__

test_oE '$# in function'
func() { echo $#; }
func
func 1
func 1 2
func 1 '2  2' 3
__IN__
0
1
2
3
__OUT__

test_oE 'arguments to function'
func() { [ $# -gt 0 ] && printf '[%s]' "$@"; echo; }
func
func 1
func 1 2
func 1 '2  2' 3
set a
func 1
echo "$@"
__IN__

[1]
[1][2]
[1][2  2][3]
[1]
a
__OUT__

test_oE '$0 remains unchanged while executing function'
func() { printf '%s\n' "${0##*/}"; }
func
func 1
__IN__
sh
sh
__OUT__

(
setup 'func() (exit $1)'

test_OE -e 0 'exit status of function call (0)'
func 0
__IN__

test_OE -e 1 'exit status of function call (1)'
func 1
__IN__

test_OE -e 19 'exit status of function call (19)'
func 19
__IN__

)

test_oE 'variable assigned in function remains after return'
func() {
    foo=bar
}
foo=
func
echo $foo
__IN__
bar
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
