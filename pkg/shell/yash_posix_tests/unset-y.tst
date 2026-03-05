# unset-y.tst: yash-specific test of the unset built-in

echo echo external a > a
echo echo external b > b
echo echo external c > c
echo echo external d > d
echo echo external x > x
chmod a+x a b c d x
setup 'PATH=.:$PATH'

test_oE -e 0 'deleting nothing (default)' -e
a=1
a() { echo function; }
unset
echo ${a-unset}
a
__IN__
1
function
__OUT__

test_oE -e 0 'deleting nothing (-v)' -e
a=1
a() { echo function; }
unset -v
echo ${a-unset}
a
__IN__
1
function
__OUT__

test_oE -e 0 'deleting nothing (-f)' -e
a=1
a() { echo function; }
unset -f
echo ${a-unset}
a
__IN__
1
function
__OUT__

test_oE -e 0 'deleting existing variable (--variables)' -e
a=1 b=2
unset --variables a
echo ${a-unset} ${b-unset}
__IN__
unset 2
__OUT__

test_oE -e 0 'deleting non-existing variable (--variables)' -e
a=1 b=2
unset --variables x
echo ${a-unset} ${b-unset} ${x-unset}
__IN__
1 2 unset
__OUT__

test_oE -e 0 'deleting many variables (--variables)' -e
a=1 b=2 c=3 d=4
unset --variables a b x c
echo ${a-unset} ${b-unset} ${c-unset} ${d-unset} ${x-unset}
__IN__
unset unset unset 4 unset
__OUT__

test_oE -e 0 'only variable is deleted (--variables)' -e
a() { echo "$@"; }
a=1
unset --variables a
a ${a-unset}
__IN__
unset
__OUT__

test_oE -e 0 'deleting array variable (--variables)' -e
a=()
unset --variables a
echo ${a-unset}
__IN__
unset
__OUT__

test_oE -e 0 'deleting local variable (--variables)' -e
f() {
    typeset a=local
    unset a
    echo $a
}
a=global
f
echo $a
__IN__
global
global
__OUT__

test_oE -e 0 'deleting existing function (--functions)' -e
a() { echo a; }
b() { echo b; }
unset --functions b
a
b
__IN__
a
external b
__OUT__

test_oE -e 0 'deleting non-existing function (--functions)' -e
a() { echo a; }
unset --functions b
a
b
__IN__
a
external b
__OUT__

test_oE -e 0 'deleting many functions (--functions)' -e
a() { echo a; }
b() { echo b; }
c() { echo c; }
d() { echo d; }
unset --functions a b x c
a
b
c
d
x
__IN__
external a
external b
external c
d
external x
__OUT__

test_oE -e 0 'only function is deleted (--functions)' -e
a=1
a() { echo a; }
unset --functions a
echo ${a-unset}
__IN__
1
__OUT__

test_oE -e 0 'function is not deleted by default' -e
a() { echo function; }
unset a
a
__IN__
function
__OUT__

test_oE -e 0 'last-specified option takes effect (-v)' -e
a=1
a() { echo function; }
unset -fv a
echo ${a-unset}
a
__IN__
unset
function
__OUT__

test_oE -e 0 'last-specified option takes effect (-f)' -e
a=1
a() { echo function; }
unset -vf a
echo ${a-unset}
a
__IN__
1
external a
__OUT__

test_oE -e 0 'invalid variable names are ignored' -e
set 1 2 3
unset = =foo
echo "$@"
__IN__
1 2 3
__OUT__

test_oE 'deleting function with non-portable name' -e
function f=/\'g() { }
unset -f f=/\'g
command -fv f=/\'g || echo function unset
__IN__
function unset
__OUT__

test_Oe -e 1 'deleting read-only variable'
readonly a=1
unset a
__IN__
unset: $a is read-only
__ERR__

test_oe 'deleting read-only function'
func() { echo func; }
readonly -f func
unset -f func
echo $?
func
__IN__
1
func
__OUT__
unset: function `func' is read-only
__ERR__
#'
#`

test_Oe -e 2 'invalid option -z'
unset -z
__IN__
unset: `-z' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
unset --no-such=option
__IN__
unset: `--no-such=option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
