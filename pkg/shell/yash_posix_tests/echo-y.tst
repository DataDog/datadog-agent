# echo-y.tst: yash-specific test of the echo built-in

setup -d

test_oE 'basic functionality of echo'
echo 123 456 789
echo 1 22 '3  3' "4
4" 5\	5
__IN__
123 456 789
1 22 3  3 4
4 5	5
__OUT__

if ! testee -c 'command -bv echo' >/dev/null; then
    skip="true"
fi

test_n_ignored() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} -n is ignored" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -n test new
echo line
__IN__
-n test new
line
__OUT__
}

test_n_recognized() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} -n is recognized" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -n test new
echo line
__IN__
test newline
__OUT__
}

test_e_ignored() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} -e is ignored" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -e foo
echo -E foo
__IN__
-e foo
-E foo
__OUT__
}

test_e_recognized() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} -E/-e is recognized" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -E '1\a2\b3\c4' 5
echo -E '6\e7\f8\n9\r0\t1\v2'
echo -E 'a\\b'
echo -E '\0123\012\01x' '\123\12\1x' '\00411'
echo -e '1\a2\b3\c4' 5
echo -e '6\e7\f8\n9\r0\t1\v2'
echo -e 'a\\b'
echo -e '\0123\012\01x' '\123\12\1x' '\00411'
__IN__
1\a2\b3\c4 5
6\e7\f8\n9\r0\t1\v2
a\\b
\0123\012\01x \123\12\1x \00411
123678
90	12
a\b
S
x \123\12\1x !1
__OUT__
}

test_e_n_combination() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} combination of -E/-e/-n" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -e -E '-e -E [\t]'
echo -E -e '-E -e [\t]'
echo -e -n -E '-e -n -E [\t]'
echo -E -n -e '-E -n -e [\t]'
echo !
echo -eE '-eE [\t]'
echo -Ee '-Ee [\t]'
echo -enE '-enE [\t]'
echo -Ene '-Ene [\t]'
echo !
__IN__
-e -E [\t]
-E -e [	]
-e -n -E [\t]-E -n -e [	]!
-eE [\t]
-Ee [	]
-enE [\t]-Ene [	]!
__OUT__
}

test_escape_default_enabled() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} escapes are enabled by default" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo '1\a2\b3\c4' 5
echo '6\f7\n8\r9\t0\v!'
echo '\0123\012\01x' '\123\12\1x' '\00411'
__IN__
12367
89	0!
S
x \123\12\1x !1
__OUT__
}

test_escape_default_disabled() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} escapes are disabled by default" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo '1\a2\b3\c4' 5
echo '6\f7\n8\r9\t0\v!'
echo '\0123\012\01x' '\123\12\1x' '\00411'
__IN__
1\a2\b3\c4 5
6\f7\n8\r9\t0\v!
\0123\012\01x \123\12\1x \00411
__OUT__
}

test_single_hyphen() {
testcase "$1" "ECHO_STYLE=${ECHO_STYLE-} single hyphen is not an option" \
    3<<\__IN__ 4<<\__OUT__ 5</dev/null
echo -
echo - -
echo - - foo
echo foo - bar
__IN__
-
- -
- - foo
foo - bar
__OUT__
}

(
unset ECHO_STYLE
test_n_ignored "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_enabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=SYSV
test_n_ignored "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_enabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=XSI
test_n_ignored "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_enabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=BSD
test_n_recognized "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_disabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=GNU
test_n_recognized "$LINENO"
test_e_recognized "$LINENO"
test_e_n_combination "$LINENO"
test_escape_default_disabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=ZSH
test_n_recognized "$LINENO"
test_e_recognized "$LINENO"
test_e_n_combination "$LINENO"
test_escape_default_enabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=DASH
test_n_recognized "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_enabled "$LINENO"
test_single_hyphen "$LINENO"
)

(
export ECHO_STYLE=RAW
test_n_ignored "$LINENO"
test_e_ignored "$LINENO"
test_escape_default_disabled "$LINENO"
test_single_hyphen "$LINENO"
)

test_O -d -e n 'echoing to closed stream'
echo >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
