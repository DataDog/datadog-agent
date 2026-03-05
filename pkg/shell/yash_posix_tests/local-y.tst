# local-y.tst: yash-specific test of the local built-in

# Since "local" is an alias for "typeset", most tests in this file are
# analogous to those in typeset-y.tst.

test_oE -e 0 'local is an elective built-in'
command -V local
__IN__
local: an elective built-in
__OUT__

test_oE -e 0 'defining variable in global namespace' -e
local a=1
echo $a
__IN__
1
__OUT__

test_oE -e 0 'defining local variable' -e
f() {
    local a=1
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

test_oE -e 0 'only local variables are printed by default (no option)' -e
f() {       a=1; local; }
g() { local a=1; local; }
f
echo ---
g
__IN__
---
local a=1
__OUT__

test_oE -e 0 'defining and printing local array (no option)' -e
f() {
    local a
    a=(This is my array.)
    printf '%s\n' "$a"
    local
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
local a
global
__OUT__

test_oE 'defining exported variables (-x)' -e
f() {
a=1
local -x a b=2
echo [$a] $b
a=3
echo $a $b
sh -c 'echo $a $b'
}
a=a b=b
f
echo $a $b
__IN__
[] 2
3 2
3 2
1 b
__OUT__

test_oE -e 0 'only local variables are printed by default (-p)' -e
f() {       a=1; local -p; }
g() { local a=1; local -p; }
f
echo ---
g
__IN__
---
local a=1
__OUT__

test_Oe -e 2 'invalid option -f'
local -f
__IN__
local: `-f' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option -g'
local -g
__IN__
local: `-g' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
local --global
__IN__
local: `--global' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'specifying -x and -X at once'
local -xX
__IN__
local: the -x option cannot be used with the -X option
__ERR__

test_O -d -e 127 'local built-in is unavailable in POSIX mode' --posix
echo echo not reached > local
chmod a+x local
PATH=$PWD:$PATH
local --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
