# set-p.tst: test of the set built-in for any POSIX-compliant shell

posix="true"

setup -d

test_oE 'setting one positional parameter (no --)' -e
set foo
bracket "$@"
__IN__
[foo]
__OUT__

test_oE 'setting three positional parameters (no --)' -e
set foo 'B  A  R' baz
bracket "$@"
__IN__
[foo][B  A  R][baz]
__OUT__

test_oE 'setting empty positional parameters (no --)' -e
set '' ''
bracket "$@"
__IN__
[][]
__OUT__

test_oE 'setting zero positional parameters' -es 1 2 3
set --
echo $#
__IN__
0
__OUT__

test_oE 'setting three positional parameters (with --)' -e
set -- - -- baz
bracket "$@"
__IN__
[-][--][baz]
__OUT__

# $1 = $LINENO, $2 = short option, $3 = long option
test_short_option_on() {
    testcase "$1" -e 0 "$3 (short) on: \$-" 3<<__IN__ 4<&- 5<&-
set -$2 &&
printf '%s\n' "\$-" | grep -q $2
__IN__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_short_option_off() {
    testcase "$1" -e 0 "$3 (short) off: \$-" "-$2" 3<<__IN__ 4<&- 5<&-
set +$2 &&
printf '%s\n' "\$-" | grep -qv $2
__IN__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_long_option_on() {
    testcase "$1" -e 0 "$3 (long) on: \$-" 3<<__IN__ 4<&- 5<&-
set -o $3 &&
printf '%s\n' "\$-" | grep -q $2
__IN__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_long_option_off() {
    testcase "$1" -e 0 "$3 (long) off: \$-" "-$2" 3<<__IN__ 4<&- 5<&-
set +o $3 &&
printf '%s\n' "\$-" | grep -qv $2
__IN__
}

test_short_option_on  "$LINENO" a allexport
test_short_option_off "$LINENO" a allexport
test_long_option_on   "$LINENO" a allexport
test_long_option_off  "$LINENO" a allexport

test_short_option_on  "$LINENO" b notify
test_short_option_off "$LINENO" b notify
test_long_option_on   "$LINENO" b notify
test_long_option_off  "$LINENO" b notify

test_short_option_on  "$LINENO" C noclobber
test_short_option_off "$LINENO" C noclobber
test_long_option_on   "$LINENO" C noclobber
test_long_option_off  "$LINENO" C noclobber

test_short_option_on  "$LINENO" e errexit
test_short_option_off "$LINENO" e errexit
test_long_option_on   "$LINENO" e errexit
test_long_option_off  "$LINENO" e errexit

test_short_option_on  "$LINENO" f noglob
test_short_option_off "$LINENO" f noglob
test_long_option_on   "$LINENO" f noglob
test_long_option_off  "$LINENO" f noglob

test_short_option_on  "$LINENO" h hashondef
test_short_option_off "$LINENO" h hashondef
# This is not POSIX.
#test_long_option_on   "$LINENO" h hashondef
#test_long_option_off  "$LINENO" h hashondef

# The -m option cannot be tested here due to dependency on the terminal.

test_short_option_on  "$LINENO" n noexec
test_short_option_off "$LINENO" n noexec
# One can never reset the -n option
#test_long_option_on   "$LINENO" n noexec
#test_long_option_off  "$LINENO" n noexec

test_short_option_on  "$LINENO" u nounset
test_short_option_off "$LINENO" u nounset
test_long_option_on   "$LINENO" u nounset
test_long_option_off  "$LINENO" u nounset

test_short_option_on  "$LINENO" v verbose
test_short_option_off "$LINENO" v verbose
test_long_option_on   "$LINENO" v verbose
test_long_option_off  "$LINENO" v verbose

test_short_option_on  "$LINENO" x xtrace
test_short_option_off "$LINENO" x xtrace
test_long_option_on   "$LINENO" x xtrace
test_long_option_off  "$LINENO" x xtrace

test_x -e 0 'setting many shell options at once' -a
set -ex +a -o noclobber -u
printf '%s\n' "$-" | grep -qv a &&
printf '%s\n' "$-" | grep C | grep e | grep u | grep -q x
__IN__

test_oE 'setting only options does not change positional parameters' -s 1 foo
set -e
bracket "$@"
__IN__
[1][foo]
__OUT__

test_oE 'setting positional parameters and shell options at once'
set -a -e foo 2
printf '%s\n' "$-" | grep a | grep -q e && bracket "$@"
__IN__
[foo][2]
__OUT__

# This test assumes that the output from "set -o" is always the same for the
# same option configuration.
test_OE -e 0 'set -o/+o'
set -aeu
set -o > saveset
saveset=$(set +o)
set +aeu -f
eval "$saveset"
set -o | diff saveset -
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
