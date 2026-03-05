# typeset-y.tst: yash-specific test of the typeset built-in

test_oE -e 0 'typeset is an elective built-in'
command -V typeset
__IN__
typeset: an elective built-in
__OUT__

test_oE -e 0 'defining variable in global namespace' -e
typeset a=1
echo $a
__IN__
1
__OUT__

test_oE -e 0 'defining local variable' -e
f() {
    typeset a=1
    b=2
    echo $a $b
    a=3 b=4
    echo $a $b
}
f
echo ${a-unset} ${b-unset}
__IN__
1 2
3 4
unset 4
__OUT__

test_oE -e 0 'overwriting temporary variable' -e
a=1 typeset a=2
echo $a
__IN__
2
__OUT__

test_oE -e 0 'redeclaring temporary variable' -e
a=1
a=2 typeset a
echo $a
__IN__
1
__OUT__

test_oE -e 0 'declaring local variable with temporary variable' -e
a=1 typeset b
echo a=${a-unset} b=${b-unset}
__IN__
a=unset b=unset
__OUT__

test_oE -e 0 'printing all variables (no option)' -e
typeset >/dev/null
typeset | grep -q '^typeset -x PATH='
yash_typeset_test=foo
typeset | grep -Fx "typeset yash_typeset_test=foo"
readonly yash_readonly_test=bar
export yash_export_test=baz
typeset | grep -Fx "typeset -r yash_readonly_test=bar"
typeset | grep -Fx "typeset -x yash_export_test=baz"
__IN__
typeset yash_typeset_test=foo
typeset -r yash_readonly_test=bar
typeset -x yash_export_test=baz
__OUT__

test_oE -e 0 'only local variables are printed by default (no option)' -e
f() {         a=1; typeset; }
g() { typeset a=1; typeset; }
f
echo ---
g
__IN__
---
typeset a=1
__OUT__

test_oE 'printing all variables (-g)'
f() {
    yash_typeset_test_a=1
    typeset yash_typeset_test_b=2
    typeset -g
}
yash_typeset_test_g=3
f | grep '^typeset.* yash_typeset_test_.='
__IN__
typeset yash_typeset_test_a=1
typeset yash_typeset_test_b=2
typeset yash_typeset_test_g=3
__OUT__

test_oE -e 0 'defining and printing local array (no option)' -e
f() {
    typeset a
    a=(This is my array.)
    printf '%s\n' "$a"
    typeset
}
a=global
f
echo $a
__IN__
This
is
my
array.
a=(This is my array.)
typeset a
global
__OUT__

test_oE 'defining read-only variables (-r)' -e
a=1
typeset -r a b=2
(typeset a=X 2>/dev/null || echo $a)
(typeset b=Y 2>/dev/null || echo $b)
__IN__
1
2
__OUT__

test_oE 'defining exported variables (-x)' -e
a=1
typeset -x a b=2
echo $a $b
sh -c 'echo $a $b'
__IN__
1 2
1 2
__OUT__

(
export a=1 b=2

test_oE 'canceling exportation of variables (-X)' -e
typeset -X a b=3
echo $a $b
sh -c 'echo ${a-unset} ${b-unset}'
__IN__
1 3
unset unset
__OUT__

)

test_oE -e 0 'printing all variables (-p)' -e
typeset -p >/dev/null
typeset -p | grep -q '^typeset -x PATH='
yash_typeset_test=foo
typeset -p | grep -Fx "typeset yash_typeset_test=foo"
readonly yash_readonly_test=bar
export yash_export_test=baz
typeset -p | grep -Fx "typeset -r yash_readonly_test=bar"
typeset -p | grep -Fx "typeset -x yash_export_test=baz"
__IN__
typeset yash_typeset_test=foo
typeset -r yash_readonly_test=bar
typeset -x yash_export_test=baz
__OUT__

test_oE -e 0 'only local variables are printed by default (-p)' -e
f() {         a=1; typeset -p; }
g() { typeset a=1; typeset -p; }
f
echo ---
g
__IN__
---
typeset a=1
__OUT__

test_oE -e 0 'printing specific variables (-p)' -e
a=1 b=2 c=3
typeset -p a b
__IN__
typeset a=1
typeset b=2
__OUT__

test_oE -e 0 'printing array variable (-p)' -e
a=() b=(1 '2  2' 3)
typeset -x b
typeset -p a b
__IN__
a=()
typeset a
b=(1 '2  2' 3)
typeset -x b
__OUT__

test_oE -e 0 'assigning variable with -p' -e
a=1
typeset -p a b=2
echo $a $b
__IN__
typeset a=1
1 2
__OUT__

test_oE 'printing all variables (-gp)'
f() {
    yash_typeset_test_a=1
    typeset yash_typeset_test_b=2
    typeset -gp
}
yash_typeset_test_g=3
f | grep '^typeset.* yash_typeset_test_.='
__IN__
typeset yash_typeset_test_a=1
typeset yash_typeset_test_b=2
typeset yash_typeset_test_g=3
__OUT__

test_oE -e 0 'printing read-only variables (-rp)' -e
typeset -r a=1
b=2
typeset -rp a b
__IN__
typeset -r a=1
__OUT__

test_oE -e n 'defining read-only variables (-rp)' -e
typeset -rp a=1
echo $a
{ a=X; } 2>/dev/null && test "$a" = 1
__IN__
1
__OUT__

test_oE -e 0 'printing exported variables (-xp)' -e
typeset -x a=1
b=2
typeset -xp a b
__IN__
typeset -x a=1
__OUT__

test_oE -e 0 'defining exported variables (-xp)' -e
typeset -xp a=1
echo $a
sh -c 'echo $a'
__IN__
1
1
__OUT__

test_oE -e 0 'printing variables: -X is ignored with -p (-Xp)' -e
a=1
typeset -x b=2
typeset -Xp a b
__IN__
typeset a=1
typeset -x b=2
__OUT__

test_oE -e 0 'printing global exported variables (-gxp)' -e
g=1
typeset -x h=2
func() {
    typeset l=3
    typeset -x m=4
    typeset -gxp g h l m
}
func
__IN__
typeset -x h=2
typeset -x m=4
__OUT__

test_oE 'defining global exported variables (-gxp)' -e
func() { typeset -gxp a=1; }
func
sh -c 'echo $a'
__IN__
1
__OUT__

test_oE -e 0 'printing read-only exported variables (-rxp)' -e
typeset n=neither
typeset -r r=readonly
typeset -x x=exported
typeset -rx b=both
typeset -rxp n r x b
__IN__
typeset -xr b=both
__OUT__

test_oE 'defining read-only exported variables (-rxp)' -e
typeset -rxp a=1
sh -c 'echo $a'
(typeset a=X 2>/dev/null || echo $a)
__IN__
1
1
__OUT__

test_oE -e 0 'printing read-only variables: -X is ignored with -p (-rXp)' -e
typeset -r a=1
b=2
typeset -rXp a b
__IN__
typeset -r a=1
__OUT__

test_oE -e n 'defining read-only un-exported variables (-rXp)' -e
typeset -x a=0
typeset -rXp a=1
echo $a
sh -c 'echo ${a-unset}'
{ a=X; } 2>/dev/null && test "$a" = 1
__IN__
1
unset
__OUT__

test_x -e 0 'printing all functions (-f): exit status' -e
f() { }
g() for i in 1; do echo $i; done
typeset -f
__IN__

test_oE 'printing all functions (-f): output' -e
f() { }
g() for i in 1; do echo $i; done
typeset -f | sed 's;^[[:space:]]*;;'
__IN__
f()
{
}
g()
for i in 1
do
echo ${i}
done
__OUT__

test_OE -e 0 'testing existence of functions (-f)' -e
f() { }
g() for i in 1; do echo $i; done
typeset -f f g
__IN__

test_oe -e n 'making function readonly (-fr)' -e
f() { echo f; }
g() { echo g; }
typeset -fr f
f
g
eval 'f() { }'
__IN__
f
g
__OUT__
eval: function `f' cannot be redefined because it is read-only
__ERR__
#'
#`

test_x -e 0 'printing all functions (-fp): exit status' -e
f() { }
g() for i in 1; do echo $i; done
typeset -fp
__IN__

test_oE 'printing all functions (-fp): output' -e
f() { }
g() for i in 1; do echo $i; done
typeset -fp | sed 's;^[[:space:]]*;;'
__IN__
f()
{
}
g()
for i in 1
do
echo ${i}
done
__OUT__

test_oE -e 0 'printing specific functions (-fp)' -e
f() { }
g() ( )
h() for i in 1; do echo $i; done
typeset -fp f g
__IN__
f()
{
}
g()
()
__OUT__

test_oE -e 0 'printing function with non-portable name (-fp)' -e
function f=/\'g() { }
typeset -fp "f=/'g"
__IN__
function 'f=/'\'g()
{
}
__OUT__

test_oE 'printing function with command substitution with subshell (-fp)' -e
eval "$(
    print_foo() {
        echo "$((echo foo) )"
    }
    typeset -fp print_foo
)"
print_foo
__IN__
foo
__OUT__

test_oE -e 0 'printing read-only function (-frp)' -e
f() { }
g() ( )
typeset -fr f
typeset -frp f g
__IN__
f()
{
}
typeset -fr f
__OUT__

test_oE -e 0 'separator preceding value-less variable name starting with -' -e
typeset -g -- -a
typeset -p -- -a
__IN__
typeset -- -a
__OUT__

test_oE -e 0 'separator preceding scalar variable name starting with -' -e
typeset -g -- -a=1
typeset -p -- -a
__IN__
typeset -- -a=1
__OUT__

(
if ! testee --version --verbose | grep -Fqx ' * array'; then
    skip="true"
fi

test_oE -e 0 'separator preceding array variable name starting with -' -e
array -- -a 1 2 3
typeset -x -- -a
typeset -p -- -a
__IN__
-a=(1 2 3)
typeset -x -- -a
__OUT__

)

test_oE -e 0 'separator preceding function name starting with -' -e
function -n() { :; }
typeset -fr -- -n
typeset -fp -- -n
__IN__
function -n()
{
   :
}
typeset -fr -- -n
__OUT__

test_Oe -e 2 'invalid option -z'
typeset -z
__IN__
typeset: `-z' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
typeset --no-such=option
__IN__
typeset: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'specifying -x and -X at once'
typeset -xX
__IN__
typeset: the -x option cannot be used with the -X option
__ERR__

test_Oe -e 2 'specifying -f and -g at once'
typeset -fg
__IN__
typeset: the -f option cannot be used with the -g option
__ERR__

test_Oe -e 2 'specifying -f and -x at once'
typeset -fx
__IN__
typeset: the -f option cannot be used with the -x option
__ERR__

test_Oe -e 2 'specifying -f and -X at once'
typeset -fX
__IN__
typeset: the -f option cannot be used with the -X option
__ERR__

test_O -d -e 1 'printing to closed output stream (all variables w/o -p)'
typeset >&-
__IN__

test_O -d -e 1 'printing to closed output stream (all variables with -p)'
typeset -p >&-
__IN__

test_O -d -e 1 'printing to closed output stream (specific variable)'
typeset -p PWD >&-
__IN__

(
setup 'func() { :; }'

test_O -d -e 1 'printing to closed output stream (all functions w/o -p)'
typeset -f >&-
__IN__

test_O -d -e 1 'printing to closed output stream (all functions with -p)'
typeset -fp >&-
__IN__

test_O -d -e 1 'printing to closed output stream (specific function)'
typeset -fp func >&-
__IN__

)

test_Oe -e 1 'assigning to read-only variable'
typeset -r a
typeset a=1
__IN__
typeset: $a is read-only
__ERR__

test_Oe -e 1 'printing non-existing variable'
typeset -p a
__IN__
typeset: no such variable $a
__ERR__

test_Oe -e 1 'printing non-existing function'
typeset -fp a
__IN__
typeset: no such function `a'
__ERR__
#'
#`

test_O -d -e 127 'typeset built-in is unavailable in POSIX mode' --posix
echo echo not reached > typeset
chmod a+x typeset
PATH=$PWD:$PATH
typeset --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
