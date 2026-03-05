# set-y.tst: yash-specific test of the set built-in

# Many of the tests in this file are identical to those in set-p.tst.

setup -d

test_x -e 0 'printing all variables: exit status'
set
__IN__

test_oE 'printing all variables: output'
yashtest1='foo'
export yashtest2='"double"'
readonly yashtest3="'single'"
yashtest4='back\slash'
unset yashtest5 yashtest6
export yashtest5
readonly yashtest6
yashtest0=
set | grep '^yashtest'
__IN__
yashtest0=''
yashtest1=foo
yashtest2='"double"'
yashtest3=\'single\'
yashtest4='back\slash'
__OUT__

test_oE 'printing all variables: inside function'
yashtest1=global
f() { typeset yashtest2=local; set; }
f | grep '^yashtest'
__IN__
yashtest1=global
yashtest2=local
__OUT__

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
test_long_option_on   "$LINENO" h hashondef
test_long_option_off  "$LINENO" h hashondef

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

# $1 = $LINENO, $2 = long option
test_long_option_default_off() {
    testcase "$1" -e 0 "the $2 option is off by default" 3<<__IN__ 4<&- 5<&-
save="\$(set +o)" &&
set +o   $2 && test "\$(set +o)" =  "\$save" &&
set -o   $2 && test "\$(set +o)" != "\$save" &&
set -o no$2 && test "\$(set +o)" =  "\$save" &&
set +o no$2 && test "\$(set +o)" != "\$save" &&
set    ++$2 && test "\$(set +o)" =  "\$save" &&
set    --$2 && test "\$(set +o)" != "\$save" &&
set  --no$2 && test "\$(set +o)" =  "\$save" &&
set  ++no$2 && test "\$(set +o)" != "\$save"
__IN__
}

# $1 = $LINENO, $2 = long option
test_long_option_default_on() {
    testcase "$1" -e 0 "the $2 option is off by default" 3<<__IN__ 4<&- 5<&-
save="\$(set +o)" &&
set -o   $2 && test "\$(set +o)" =  "\$save" &&
set +o   $2 && test "\$(set +o)" != "\$save" &&
set +o no$2 && test "\$(set +o)" =  "\$save" &&
set -o no$2 && test "\$(set +o)" != "\$save" &&
set    --$2 && test "\$(set +o)" =  "\$save" &&
set    ++$2 && test "\$(set +o)" != "\$save" &&
set  ++no$2 && test "\$(set +o)" =  "\$save" &&
set  --no$2 && test "\$(set +o)" != "\$save"
__IN__
}

test_long_option_default_off "$LINENO" allexport
test_long_option_default_off "$LINENO" braceexpand
test_long_option_default_on  "$LINENO" caseglob
test_long_option_default_on  "$LINENO" clobber
test_long_option_default_on  "$LINENO" curasync
test_long_option_default_on  "$LINENO" curbg
test_long_option_default_on  "$LINENO" curstop
test_long_option_default_off "$LINENO" dotglob
test_long_option_default_off "$LINENO" emptylastfield
test_long_option_default_off "$LINENO" errexit
# One can never reset the no-exec option
#test_long_option_default_on  "$LINENO" exec
test_long_option_default_off "$LINENO" extendedglob
test_long_option_default_on  "$LINENO" glob
test_long_option_default_off "$LINENO" hashondef
test_long_option_default_off "$LINENO" ignoreeof
test_long_option_default_off "$LINENO" markdirs
# The monitor option cannot be tested here due to dependency on the terminal.
test_long_option_default_off "$LINENO" notify
test_long_option_default_off "$LINENO" nullglob
test_long_option_default_off "$LINENO" pipefail
# This needs a special test (see below)
#test_long_option_default_off "$LINENO" posixlycorrect
test_long_option_default_on  "$LINENO" traceall
test_long_option_default_on  "$LINENO" unset
test_long_option_default_off "$LINENO" verbose
test_long_option_default_off "$LINENO" xtrace

(
if ! testee --version --verbose | grep -Fqx ' * history'; then
    skip="true"
fi
test_long_option_default_off "$LINENO" histspace
)
(
if ! testee --version --verbose | grep -Fqx ' * lineedit'; then
    skip="true"
fi
test_long_option_default_off "$LINENO" emacs
test_long_option_default_off "$LINENO" lealwaysrp
test_long_option_default_off "$LINENO" lecompdebug
test_long_option_default_off "$LINENO" leconvmeta
test_long_option_default_off "$LINENO" lenoconvmeta
test_long_option_default_on  "$LINENO" lepromptsp
test_long_option_default_off "$LINENO" levisiblebell
test_long_option_default_off "$LINENO" notifyle
test_long_option_default_off "$LINENO" vi
)

test_x -e 0 'the posixlycorrect option is off by default'
save="$(set +o)" &&
set +o   posixlycorrect && test "$(set +o)" =  "$save" &&
set -o   posixlycorrect && test "$(set +o)" != "$save" &&
set -o noposixlycorrect && test "$(set +o)" =  "$save" &&
set +o noposixlycorrect && test "$(set +o)" != "$save" &&
set +o   posixlycorrect &&
set    --posixlycorrect && test "$(set +o)" != "$save" &&
set +o   posixlycorrect && test "$(set +o)" =  "$save" &&
set  ++noposixlycorrect && test "$(set +o)" != "$save"
__IN__

# $1 = $LINENO, $2 = short option, $3 = long option
test_unenablable_short_option() {
    testcase "$1" -e 2 "$3 cannot be enabled by set (short)" \
        3<<__IN__ 4</dev/null 5<<__ERR__
set -$2
__IN__
set: the $3 option cannot be changed once the shell has been initialized
__ERR__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_undisablable_short_option() {
    testcase "$1" -e 2 "$3 cannot be disabled by set (short)" \
        3<<__IN__ 4</dev/null 5<<__ERR__
set +$2
__IN__
set: the $3 option cannot be changed once the shell has been initialized
__ERR__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_unenablable_long_option() {
    testcase "$1" -e 2 "$3 cannot be enabled by set (long)" \
        3<<__IN__ 4</dev/null 5<<__ERR__
set --$3
__IN__
set: the $3 option cannot be changed once the shell has been initialized
__ERR__
}

# $1 = $LINENO, $2 = short option, $3 = long option
test_undisablable_long_option() {
    testcase "$1" -e 2 "$3 cannot be disabled by set (long)" \
        3<<__IN__ 4</dev/null 5<<__ERR__
set ++$3
__IN__
set: the $3 option cannot be changed once the shell has been initialized
__ERR__
}

test_unenablable_short_option  "$LINENO" c cmdline
test_undisablable_short_option "$LINENO" c cmdline
test_unenablable_long_option   "$LINENO" c cmdline
test_undisablable_long_option  "$LINENO" c cmdline

test_unenablable_short_option  "$LINENO" i interactive
test_undisablable_short_option "$LINENO" i interactive
test_unenablable_long_option   "$LINENO" i interactive
test_undisablable_long_option  "$LINENO" i interactive

test_unenablable_short_option  "$LINENO" l login
test_undisablable_short_option "$LINENO" l login
test_unenablable_long_option   "$LINENO" l login
test_undisablable_long_option  "$LINENO" l login

test_unenablable_short_option  "$LINENO" s stdin
test_undisablable_short_option "$LINENO" s stdin
test_unenablable_long_option   "$LINENO" s stdin
test_undisablable_long_option  "$LINENO" s stdin

test_oE -e 0 'option name is case-insensitive'
set -o   AllExPort && set +o | head -n 1 &&
set +o   aLLeXpORT && set +o | head -n 1 &&
set +o NoAllExPort && set +o | head -n 1 &&
set -o nOaLLeXpORT && set +o | head -n 1 &&
set    --aLLeXpORT && set +o | head -n 1 &&
set    ++AllExPort && set +o | head -n 1 &&
set  ++NoaLLeXpORT && set +o | head -n 1 &&
set  --nOAllExPort && set +o | head -n 1
__IN__
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
__OUT__

test_oE -e 0 'symbols in option name are ignored'
set -o    allexpor-t && set +o | head -n 1 &&
set +o    allexpo:rt && set +o | head -n 1 &&
set +o n^oallexp-ort && set +o | head -n 1 &&
set -o no#allex+port && set +o | head -n 1 &&
set     --alle-xport && set +o | head -n 1 &&
set     ++all_export && set +o | head -n 1 &&
set  ++!noal-lexport && set +o | head -n 1 &&
set  --n@oa_llexport && set +o | head -n 1
__IN__
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
__OUT__

test_oE -e 0 'option name can be abbreviated'
set -o   allexpor && set +o | head -n 1 &&
set +o   allexpo && set +o | head -n 1 &&
set +o noallexp && set +o | head -n 1 &&
set -o noallex && set +o | head -n 1 &&
set    --alle && set +o | head -n 1 &&
set    ++all && set +o | head -n 1 &&
set  ++noal && set +o | head -n 1 &&
set  --noa && set +o | head -n 1
__IN__
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
set -o allexport
set +o allexport
__OUT__

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

test_x -e 0 'set -o: exit status'
set -o
__IN__

test_oE 'set -o: output'
set -o | grep -v '^histspace ' |
grep -v '^le' | grep -v '^emacs ' | grep -v '^notifyle ' | grep -v '^vi '
echo ---
set -a +o caseglob -o dotglob
set -o | head -n 9
__IN__
allexport       off
braceexpand     off
caseglob        on
clobber         on
cmdline         off
curasync        on
curbg           on
curstop         on
dotglob         off
emptylastfield  off
errexit         off
errreturn       off
exec            on
extendedglob    off
forlocal        on
glob            on
hashondef       off
ignoreeof       off
interactive     off
log             on
login           off
markdirs        off
monitor         off
notify          off
nullglob        off
pipefail        off
posixlycorrect  off
stdin           on
traceall        on
unset           on
verbose         off
xtrace          off
---
allexport       on
braceexpand     off
caseglob        off
clobber         on
cmdline         off
curasync        on
curbg           on
curstop         on
dotglob         on
__OUT__

test_E -e 0 'set -o: sorted'
set -o | sort -c
__IN__

test_x -e 0 'set +o: exit status'
set +o
__IN__

test_oE 'set +o: output'
set +o |
grep -v '^set [+-]o le' |
grep -Fvx 'set +o emacs' |
grep -Fvx 'set +o histspace' |
grep -Fvx 'set +o notifyle' |
grep -Fvx 'set +o vi'
__IN__
set +o allexport
set +o braceexpand
set -o caseglob
set -o clobber
set -o curasync
set -o curbg
set -o curstop
set +o dotglob
set +o emptylastfield
set +o errexit
set +o errreturn
set -o exec
set +o extendedglob
set -o forlocal
set -o glob
set +o hashondef
set +o ignoreeof
set -o log
set +o markdirs
set +o monitor
set +o notify
set +o nullglob
set +o pipefail
set +o posixlycorrect
set -o traceall
set -o unset
set +o verbose
set +o xtrace
__OUT__

test_Oe -e 2 'invalid option (short, only available in startup)'
set -V
__IN__
set: `V' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option (short, hyphen)'
set -C-
__IN__
set: `-' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option (short, unknown)'
set -aXb
__IN__
set: `X' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option (long)'
set --version
__IN__
set: `--version' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'missing option argument'
set -o allexport -o
__IN__
set: the -o option requires an argument
__ERR__

test_O -e 2 'ambiguous option: standard output and exit status'
set --cu
__IN__

test_o 'ambiguous option: error message'
set --cu 2>&1 | head -n 1
__IN__
set: option `--cu' is ambiguous
__OUT__
#'
#`

test_O -d -e 2 'ambiguous option (with and without "no"-prefix)'
set --not
__IN__

test_O -d -e 1 'printing to closed stream'
set >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
