# option-p.tst: test of shell options for any POSIX-compliant shell

posix="true"

test_x -e 0 'allexport (short) on: $-' -a
printf '%s\n' "$-" | grep -q a
__IN__

test_x -e 0 'allexport (long) on: $-' -o allexport
printf '%s\n' "$-" | grep -q a
__IN__

test_x -e 0 'allexport (short) off: $-' +a
printf '%s\n' "$-" | grep -qv a
__IN__

test_x -e 0 'allexport (long) off: $-' +o allexport
printf '%s\n' "$-" | grep -qv a
__IN__

test_oE 'allexport (short) on: effect' -a
unset foo
foo=bar
sh -c 'echo ${foo-unset}'
__IN__
bar
__OUT__

test_oE 'allexport (long) on: effect' -o allexport
unset foo
foo=bar
sh -c 'echo ${foo-unset}'
__IN__
bar
__OUT__

test_oE 'allexport (short) off: effect' +a
unset foo
foo=bar
sh -c 'echo ${foo-unset}'
__IN__
unset
__OUT__

test_oE 'allexport (long) off: effect' +o allexport
unset foo
foo=bar
sh -c 'echo ${foo-unset}'
__IN__
unset
__OUT__

# XXX: test of the -b option is not supported

test_x -e 0 'errexit (short) on: $-' -e
printf '%s\n' "$-" | grep -q e
__IN__

test_x -e 0 'errexit (long) on: $-' -o errexit
printf '%s\n' "$-" | grep -q e
__IN__

test_x -e 0 'errexit (short) off: $-' +e
printf '%s\n' "$-" | grep -qv e
__IN__

test_x -e 0 'errexit (long) off: $-' +o errexit
printf '%s\n' "$-" | grep -qv e
__IN__

# Other tests of the -e option are in errexit-p.tst.

test_x -e 0 'noglob (short) on: $-' -f
printf '%s\n' "$-" | grep -q f
__IN__

test_x -e 0 'noglob (long) on: $-' -o noglob
printf '%s\n' "$-" | grep -q f
__IN__

test_x -e 0 'noglob (short) off: $-' +f
printf '%s\n' "$-" | grep -qv f
__IN__

test_x -e 0 'noglob (long) off: $-' +o noglob
printf '%s\n' "$-" | grep -qv f
__IN__

test_oE 'noglob (short) on: effect' -f
echo /*
__IN__
/*
__OUT__

test_oE 'noglob (long) on: effect' -o noglob
echo /*
__IN__
/*
__OUT__

test_oE 'noglob (short) off: effect' +f
printf '%s\n' /* | grep -x /dev
__IN__
/dev
__OUT__

test_oE 'noglob (long) off: effect' +o noglob
printf '%s\n' /* | grep -x /dev
__IN__
/dev
__OUT__

test_x -e 0 'hashondef (short) on: $-' -h
printf '%s\n' "$-" | grep -q h
__IN__

test_x -e 0 'hashondef (short) off: $-' +h
printf '%s\n' "$-" | grep -qv h
__IN__

# XXX: test of the -m option is not supported

test_x -e 0 'noexec (short) on: $-' -n
printf '%s\n' "$-" | grep -q n
__IN__

test_x -e 0 'noexec (long) on: $-' -o noexec
printf '%s\n' "$-" | grep -q n
__IN__

test_x -e 0 'noexec (short) off: $-' +n
printf '%s\n' "$-" | grep -qv n
__IN__

test_x -e 0 'noexec (long) off: $-' +o noexec
printf '%s\n' "$-" | grep -qv n
__IN__

test_OE 'noexec (short) on: simple command is not executed' -n
echo executed
__IN__

test_OE 'noexec (long) on: simple command is not executed' -o noexec
echo executed
__IN__

test_oE 'noexec (short) off: simple command is executed' +n
echo executed
__IN__
executed
__OUT__

test_oE 'noexec (long) off: simple command is executed' +o noexec
echo executed
__IN__
executed
__OUT__

{
testee -cn 'for i in $(>noexec_file); do :; done'

test_OE -e 0 'noexec (short) on: for command is not executed'
! [ -e noexec_file ]
__IN__

}

# See pipeline-p.tst for the pipefail option tests.

test_x -e 0 'nounset (short) on: $-' -u
printf '%s\n' "$-" | grep -q u
__IN__

test_x -e 0 'nounset (long) on: $-' -o nounset
printf '%s\n' "$-" | grep -q u
__IN__

test_x -e 0 'nounset (short) off: $-' +u
printf '%s\n' "$-" | grep -qv u
__IN__

test_x -e 0 'nounset (long) off: $-' +o nounset
printf '%s\n' "$-" | grep -qv u
__IN__

(
setup -d
setup 'foo=bar s=; unset x'

test_oE -e 0 'nounset on: expansions of set variable' -u
bracket ${foo} ${foo-unset} ${foo:-unset} ${foo+set} ${foo:+set}
bracket "${s}" "${s-unset}" "${s:-unset}" "${s+set}" "${s:+set}"
bracket ${#foo} ${#s}
bracket ${foo#b} ${foo##b} ${foo%r} ${foo%%r}
bracket "${s#b}" "${s##b}" "${s%r}" "${s%%r}"
__IN__
[bar][bar][bar][set][set]
[][][unset][set][]
[3][0]
[ar][ar][ba][ba]
[][][][]
__OUT__

test_oE -e 0 'nounset on: set variable ${foo=bar}' -u
bracket ${foo=X}
bracket ${foo}
__IN__
[bar]
[bar]
__OUT__

test_oE -e 0 'nounset on: set variable ${foo:=bar}' -u
bracket ${foo:=X}
bracket ${foo}
__IN__
[bar]
[bar]
__OUT__

test_oE -e 0 'nounset on: set variable ${foo?bar}' -u
bracket ${foo?X}
__IN__
[bar]
__OUT__

test_oE -e 0 'nounset on: set variable ${foo:?bar}' -u
bracket ${foo:?X}
__IN__
[bar]
__OUT__

test_oE -e 0 'nounset on: set variable $((foo))' -u
bracket $((x=42))
bracket $((x))
__IN__
[42]
[42]
__OUT__

test_oE -e 0 'nounset on: empty variable ${foo=bar}' -u
bracket "${s=X}"
bracket "${s}"
__IN__
[]
[]
__OUT__

test_oE -e 0 'nounset on: empty variable ${foo:=bar}' -u
bracket "${s:=X}"
bracket "${s}"
__IN__
[X]
[X]
__OUT__

test_oE -e 0 'nounset on: empty variable ${foo?bar}' -u
bracket "${s?X}"
__IN__
[]
__OUT__

test_O -d -e n 'nounset on: empty variable ${foo:?bar}' -u
bracket "${s:?X}"
bracket "${s}"
__IN__

test_O -d -e n 'nounset on: unset variable ${foo}' -u
bracket ${x}
__IN__

test_oE -e 0 'nounset on: unset variable ${foo-bar}' -u
bracket ${x-unset}
__IN__
[unset]
__OUT__

test_oE -e 0 'nounset on: unset variable ${foo:-bar}' -u
bracket ${x:-unset}
__IN__
[unset]
__OUT__

test_oE -e 0 'nounset on: unset variable ${foo+bar}' -u
bracket "${x+set}"
__IN__
[]
__OUT__

test_oE -e 0 'nounset on: unset variable ${foo:+bar}' -u
bracket "${x:+set}"
__IN__
[]
__OUT__

test_oE -e 0 'nounset on: unset variable ${foo=bar}' -u
bracket ${x=unset}
bracket ${x}
__IN__
[unset]
[unset]
__OUT__

test_oE -e 0 'nounset on: unset variable ${foo:=bar}' -u
bracket ${x:=unset}
bracket ${x}
__IN__
[unset]
[unset]
__OUT__

test_O -d -e n 'nounset on: unset variable ${foo?bar}' -u
bracket ${x?unset}
__IN__

test_O -d -e n 'nounset on: unset variable ${foo:?bar}' -u
bracket ${x:?unset}
__IN__

test_O -d -e n 'nounset on: unset variable ${#foo}' -u
bracket "${#x}"
__IN__

test_O -d -e n 'nounset on: unset variable ${foo#bar}' -u
bracket "${x#y}"
__IN__

test_O -d -e n 'nounset on: unset variable ${foo##bar}' -u
bracket "${x##y}"
__IN__

test_O -d -e n 'nounset on: unset variable ${foo%bar}' -u
bracket "${x%y}"
__IN__

test_O -d -e n 'nounset on: unset variable ${foo%%bar}' -u
bracket "${x%%y}"
__IN__

test_O -d -e n 'nounset on: unset variable $((foo))' -u
bracket "$((x))"
__IN__

)

test_x -e 0 'verbose (short) on: $-' -v
printf '%s\n' "$-" | grep -q v
__IN__

test_x -e 0 'verbose (long) on: $-' -o verbose
printf '%s\n' "$-" | grep -q v
__IN__

test_x -e 0 'verbose (short) off: $-' +v
printf '%s\n' "$-" | grep -qv v
__IN__

test_x -e 0 'verbose (long) off: $-' +o verbose
printf '%s\n' "$-" | grep -qv v
__IN__

test_oe 'verbose (short) on: effect' -v
echo 1
echo 2
if true; then
echo 3
fi
__IN__
1
2
3
__OUT__
echo 1
echo 2
if true; then
echo 3
fi
__ERR__

test_oe 'verbose (long) on: effect' -o verbose
echo 1
echo 2
if true; then
echo 3
fi
__IN__
1
2
3
__OUT__
echo 1
echo 2
if true; then
echo 3
fi
__ERR__

test_x -e 0 'xtrace (short) on: $-' -x
printf '%s\n' "$-" | grep -q x
__IN__

test_x -e 0 'xtrace (long) on: $-' -o xtrace
printf '%s\n' "$-" | grep -q x
__IN__

test_x -e 0 'xtrace (short) off: $-' +x
printf '%s\n' "$-" | grep -qv x
__IN__

test_x -e 0 'xtrace (long) off: $-' +o xtrace
printf '%s\n' "$-" | grep -qv x
__IN__

test_oe 'xtrace (short) on: effect' -x
foo=bar
echo $foo
__IN__
bar
__OUT__
+ foo=bar
+ echo bar
__ERR__

test_oe 'xtrace (long) on: effect' -o xtrace
foo=bar
echo $foo
__IN__
bar
__OUT__
+ foo=bar
+ echo bar
__ERR__

test_oe '$PS4'
foo=XY PS4='${foo#X} '; set -x 2>/dev/null
echo xtrace
__IN__
xtrace
__OUT__
Y echo xtrace
__ERR__

# To test the ignoreeof option, we need to emulate the terminal device.
#test_oe 'ignoreeof on: effect' -o ignoreeof

test_OE -e 0 'ignoreeof is ignored if not interactive' +i -o ignoreeof
# The shell should exit successfully at EOF
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
