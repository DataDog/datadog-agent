# fulltest.sh: runs tests for many combinations of configuration options
# (C) 2010-2018 magicant

do_test () {
    if [ -r Makefile ]; then
        $MAKE clean
    fi
    printf '\n========== ./configure %s\n' "$*"
    ./configure "$@"
    $MAKE test
}

set -o errexit
cd -- "$(dirname -- "$0")/.."

echo "$0: using '${MAKE:=make}' as make"

a0='' a1='--disable-lineedit' a2='--disable-history --disable-lineedit'
b0='' b1='--disable-double-bracket' b2='--disable-double-bracket --disable-test'
c0='' c1='--disable-array'
d0='' d1='--disable-dirstack'
e0='' e1='--disable-help'
f0='' f1='--disable-nls'
g0='' g1='--disable-printf'
h0='' h1='--disable-socket'
i0='' i1='--disable-ulimit'
j0='' j1='--debug'

do_test $a0 $b0 $c0 $d0 $e1 $f1 $g0 $h1 $i0 $j0 "$@"
do_test $a0 $b1 $c1 $d1 $e1 $f0 $g1 $h0 $i1 $j1 "$@"
do_test $a0 $b2 $c0 $d1 $e0 $f0 $g1 $h1 $i0 $j1 "$@"
do_test $a1 $b0 $c0 $d0 $e0 $f0 $g1 $h1 $i1 $j1 "$@"
do_test $a1 $b1 $c1 $d0 $e1 $f0 $g0 $h0 $i1 $j0 "$@"
do_test $a1 $b2 $c1 $d1 $e1 $f1 $g1 $h0 $i0 $j0 "$@"
do_test $a2 $b0 $c1 $d1 $e0 $f1 $g1 $h0 $i1 $j0 "$@"
do_test $a2 $b1 $c0 $d1 $e0 $f1 $g1 $h1 $i0 $j1 "$@"
do_test $a2 $b2 $c0 $d1 $e0 $f0 $g0 $h0 $i1 $j1 "$@"
do_test $a2 $b2 $c1 $d0 $e1 $f0 $g1 $h1 $i0 $j0 "$@"
