# unset-p.tst: test of the unset built-in for any POSIX-compliant shell

posix="true"

echo echo external a > a
echo echo external b > b
echo echo external c > c
echo echo external d > d
echo echo external x > x
chmod a+x a b c d x
setup 'PATH=.:$PATH'

test_oE -e 0 'deleting existing variable (default)' -e
a=1 b=2
unset a
echo ${a-unset} ${b-unset}
__IN__
unset 2
__OUT__

test_oE -e 0 'deleting non-existing variable (default)' -e
a=1 b=2
unset x
echo ${a-unset} ${b-unset} ${x-unset}
__IN__
1 2 unset
__OUT__

test_oE -e 0 'deleting many variables (default)' -e
a=1 b=2 c=3 d=4
unset a b x c
echo ${a-unset} ${b-unset} ${c-unset} ${d-unset} ${x-unset}
__IN__
unset unset unset 4 unset
__OUT__

test_oE -e 0 'only variable is deleted by default' -e
a() { echo "$@"; }
a=1
unset a
a ${a-unset}
__IN__
unset
__OUT__

test_oE -e 0 'deleting many variables (-v)' -e
a=1 b=2 c=3 d=4
unset -v a b x c
echo ${a-unset} ${b-unset} ${c-unset} ${d-unset} ${x-unset}
__IN__
unset unset unset 4 unset
__OUT__

test_oE -e 0 'only variable is deleted (-v)' -e
a() { echo "$@"; }
a=1
unset -v a
a ${a-unset}
__IN__
unset
__OUT__

test_oE -e 0 'deleting existing function (-f)' -e
a() { echo a; }
b() { echo b; }
unset -f b
a
b
__IN__
a
external b
__OUT__

test_oE -e 0 'deleting non-existing function (-f)' -e
a() { echo a; }
unset -f b
a
b
__IN__
a
external b
__OUT__

test_oE -e 0 'deleting many functions (-f)' -e
a() { echo a; }
b() { echo b; }
c() { echo c; }
d() { echo d; }
unset -f a b x c
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

test_oE -e 0 'only function is deleted (-f)' -e
a=1
a() { echo a; }
unset -f a
echo ${a-unset}
__IN__
1
__OUT__

test_O -d -e n 'read-only variable cannot be deleted (default)'
readonly a=
unset a
echo not reached # special built-in error kills non-interactive shell
__IN__

test_O -d -e n 'read-only variable cannot be deleted (-v)'
readonly a=
unset -v a
echo not reached # special built-in error kills non-interactive shell
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
