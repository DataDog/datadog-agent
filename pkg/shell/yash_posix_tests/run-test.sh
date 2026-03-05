# run-test.sh: runs a set of test cases
# (C) 2016-2025 magicant
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
# 
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
# 
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# This script expects two operands.
# The first is the pathname to the testee, the shell that is to be tested.
# The second is the pathname to the test file that defines test cases.
# The result is output to a result file whose name is given by replacing the
# extension of the test file with ".trs".
# If any test case fails, it is also reported to the standard error.
# If the -r option is specified, intermediate files are not removed.
# If the -v option is specified, the testee is tested by Valgrind.
# The exit status is zero unless a critical error occurs. Failure of test cases
# does not cause the script to return non-zero.

set -Ceu
umask u+rwx

##### Some utility functions and aliases

eprintf() {
    printf "$@" >&2
}

# $1 = pathname
absolute()
case "$1" in
    (/*)
        printf '%s\n' "$1";;
    (*)
        printf '%s/%s' "${PWD%/}" "$1";;
esac

##### Script startup

# The -c option is not POSIXly-portable, but many shells support it.
ulimit -c 0 2>/dev/null || :

exec </dev/null 3>&- 4>&- 5>&-

# ensure correctness of $PWD
cd -L .

remove_work_dir="true"
use_valgrind="false"
while getopts rv opt; do
    case $opt in
        (r)
            remove_work_dir="false";;
        (v)
            use_valgrind="true";;
        (*)
            exit 64 # sysexits.h EX_USAGE
    esac
done
shift "$((OPTIND-1))"

testee="${1:?testee not specified}"
test_file="${2:?test file not specified}"

testee="$(absolute "$(command -v -- "$testee")")"

exec >|"${test_file%.*}.trs"

export LC_CTYPE="${LC_ALL-${LC_CTYPE-$LANG}}"
export LANG=C
export YASH_LOADPATH= # ignore default yashrc
unset -v CDPATH COLUMNS COMMAND COMMAND_NOT_FOUND_HANDLER DIRSTACK ECHO_STYLE
unset -v ENV FCEDIT HANDLED HISTFILE HISTRMDUP HISTSIZE HOME IFS LC_ALL
unset -v LC_COLLATE LC_MESSAGES LC_MONETARY LC_NUMERIC LC_TIME LINES MAIL
unset -v MAILCHECK MAILPATH NLSPATH OLDPWD POST_PROMPT_COMMAND PROMPT_COMMAND
unset -v PS1 PS1R PS1S PS2 PS2R PS2S PS3 PS3R PS3S PS4 PS4R PS4S 
unset -v RANDOM TERM XDG_CONFIG_HOME YASH_AFTER_CD YASH_LE_TIMEOUT YASH_VERSION
unset -v A B C D E F G H I J K L M N O P Q R S T U V W X Y Z _
unset -v a b c d e f g h i j k l m n o p q r s t u v w x y z
unset -v posix skip

##### Prepare temporary directory

work_dir="tmp.$$"

rm_work_dir()
if "$remove_work_dir"; then
    if [ -d "$work_dir" ]; then chmod -R a+rX "$work_dir"; fi
    rm -fr "$work_dir"
fi

trap rm_work_dir EXIT
trap 'rm_work_dir; trap - INT;  kill -INT  $$' INT
trap 'rm_work_dir; trap - TERM; kill -TERM $$' TERM
trap 'rm_work_dir; trap - QUIT; kill -QUIT $$' QUIT

mkdir "$work_dir"

##### Some more utilities

{
    if diff -U 10000 /dev/null /dev/null; then
        diff_opt='-U 10000'
    elif diff -C 10000 /dev/null /dev/null; then
        diff_opt='-C 10000'
    else
        diff_opt=''
    fi
} >/dev/null 2>&1

setup_script=""

# Add a setup script that is run before each test case
# If the first argument is "-" or omitted, the script is read from stdin.
# If the first argument is "-d", the default utility functions are added.
# Otherwise, the first argument is added as the script.
setup() {
    case "${1--}" in
        (-)
            setup "$(cat)"
            ;;
        (-d)
            setup <<\END
_empty= _sp=' ' _tab='  ' _nl='
'
echoraw() {
    printf '%s\n' "$*"
}
bracket() {
    if [ $# -gt 0 ]; then printf '[%s]' "$@"; fi
    echo
}
END
            ;;
        (*)
            setup_script="$setup_script
$1"
            ;;
    esac
}

macos_kill_workaround()
if [ "$(uname)" = Darwin ]; then
    # On macOS, kill(2) does not appear to run any signal handlers
    # synchronously, making it impossible for the shell to respond to self-sent
    # signals at a predictable time. To work around this issue, we call the kill
    # built-in in a nested subshell on macOS. The subshell and the dummy sleep
    # command generate some more SIGCHLD signals that must be handled by the
    # shell, which makes it more likely that the signal sent by the kill built-in
    # is delivered to the shell before the subshell exits.
    setup <<'__EOF__'
kill() (trap 'sleep 0' EXIT; (trap 'sleep 0' EXIT; (trap 'sleep 0' EXIT; command kill "$@")))
__EOF__
fi

# Invokes the testee.
# If the "posix" variable is defined non-empty, the testee is invoked as "sh".
# If the "use_valgrind" variable is true, Valgrind is used to run the testee,
# in which case the testee will ignore argv[0].
testee() (
    exec_testee "$@"
)
exec_testee() {
    if [ "${posix:+set}" = set ]; then
        testee="$testee_sh"
        export TESTEE="$testee"
    fi
    if ! "$use_valgrind"; then
        exec "$testee" "$@"
    else
        test -r "$abs_suppressions" || abs_suppressions=
        exec valgrind --leak-check=full --vgdb=no --log-fd=17 \
            ${abs_suppressions:+--suppressions="$abs_suppressions"} \
            --gen-suppressions=all \
            "$testee" "$@" \
            17>>"${valgrind_file-0.valgrind}"
    fi
}

# The test case runner.
#
# Contents of file descriptor 3 are passed to the standard input of a newly
# invoked testee using a temporary file.
# Contents of file descriptor 4 and 5 are compared to the actual output from
# the standard output and error of the testee, respectively, if those file
# descriptors are open. If they differ from the expected, the test case fails.
#
# The first argument is treated as the line number where the test case appears
# in the test file. As remaining arguments, options and operands may follow.
#
# If the "-d" option is specified, the test case fails unless the actual output
# to the standard error is non-empty. File descriptor 5 is ignored.
#
# If the "-e <expected_exit_status>" option is specified, the exit status of
# the testee is also checked. If the actual exit status differs from the
# expected, the test case fails. If <expected_exit_status> is "n", the expected
# is any non-zero exit status. If <expected_exit_status> is a signal name (w/o
# the SIG-prefix), the testee is expected to be killed by the signal.
#
# If the "-f" option is specified, the test result is inverted. This is useful
# for testing unimplemented features and unfixed bugs.
#
# The first operand is used as the name of the test case.
# The remaining operands are passed as arguments to the testee.
#
# If the "skip" variable is defined non-empty, the test case is skipped.
testcase() {
    test_lineno="${1:?line number unspecified}"
    shift 1
    OPTIND=1
    diagnostic_required="false"
    expected_exit_status=""
    should_succeed="true"
    while getopts de:f opt; do
        case $opt in
            (d)
                diagnostic_required="true";;
            (e)
                expected_exit_status="$OPTARG";;
            (f)
                should_succeed="false";;
            (*)
                return 64 # sysexits.h EX_USAGE
        esac
    done
    shift "$((OPTIND-1))"
    test_case_name="${1:?unnamed test case \($test_file:$test_lineno\)}"
    shift 1

    log_stdout() {
        printf '%%%%%% %s: %s:%d: %s\n' \
            "$1" "$test_file" "$test_lineno" "$test_case_name"
    }

    in_file="$test_lineno.in"
    out_file="$test_lineno.out"
    err_file="$test_lineno.err"
    valgrind_file="$test_lineno.valgrind"

    # prepare input file
    {
        if [ "$setup_script" ]; then
            printf '%s\n' "$setup_script"
        fi
        cat <&3
    } >"$in_file"
    chmod u+r "$in_file"

    if [ "${skip-}" ]; then
        log_stdout SKIPPED
        echo
        return
    fi

    if [ -e "$out_file" ]; then
        printf 'Output file %s already exists.\n' "$out_file"
        return 1
    fi
    if [ -e "$err_file" ]; then
        printf 'Output file %s already exists.\n' "$err_file"
        return 1
    fi

    # run the testee
    log_stdout START
    set +e
    # Output files are opened in append mode to ensure write atomicity.
    # To ignore a description message printed by some shells in case the testee
    # is terminated by a signal, "$err_file" must be opened in a subshell.
    (
    exec_testee "$@" <"$in_file" >>"$out_file" 2>>"$err_file" 3>&- 4>&- 5>&-
    ) 2>/dev/null
    actual_exit_status="$?"
    set -e

    chmod u+r "$out_file" "$err_file"

    failed="false"

    # check exit status
    exit_status_fail() {
        failed="true"
        if "$should_succeed"; then
            eprintf '%s:%d: %s: exit status mismatch\n' \
                "$test_file" "$test_lineno" "$test_case_name"
        fi
    }
    case "$expected_exit_status" in
        ('')
            ;;
        (n)
            printf '%% exit status: expected=non-zero actual=%d\n\n' \
                "$actual_exit_status"
            if [ "$actual_exit_status" -eq 0 ]; then
                exit_status_fail
            fi
            ;;
        ([[:alpha:]]*)
            printf '%% exit status: expected=%s ' "$expected_exit_status"
            if [ "$actual_exit_status" -le 128 ] ||
                    ! actual_signal="$(kill -l "$actual_exit_status" \
                        2>/dev/null)"; then
                printf 'actual=%d\n\n' "$actual_exit_status"
                exit_status_fail
            else
                printf 'actual=%d(%s)\n\n' \
                    "$actual_exit_status" "$actual_signal"
                if [ "$actual_signal" != "$expected_exit_status" ]; then
                    exit_status_fail
                fi
            fi
            ;;
        (*)
            printf '%% exit status: expected=%d actual=%d\n\n' \
                "$expected_exit_status" "$actual_exit_status"
            if [ "$actual_exit_status" -ne "$expected_exit_status" ]; then
                exit_status_fail
            fi
            ;;
    esac

    # check standard output
    if { <&4; } 2>/dev/null; then
        printf '%% standard output diff:\n'
        if ! diff $diff_opt - "$out_file" <&4; then
            failed="true"
            if "$should_succeed"; then
                eprintf '%s:%d: %s: standard output mismatch\n' \
                    "$test_file" "$test_lineno" "$test_case_name"
            fi
        fi
        echo
    fi

    # check standard error
    if "$diagnostic_required"; then
        printf '%% standard error (expecting non-empty output):\n'
        cat "$err_file"
        if ! [ -s "$err_file" ]; then
            failed="true"
            if "$should_succeed"; then
                eprintf '%s:%d: %s: standard error mismatch\n' \
                    "$test_file" "$test_lineno" "$test_case_name"
            fi
        fi
        echo
    elif { <&5; } 2>/dev/null; then
        printf '%% standard error diff:\n'
        if ! diff $diff_opt - "$err_file" <&5; then
            failed="true"
            if "$should_succeed"; then
                eprintf '%s:%d: %s: standard error mismatch\n' \
                    "$test_file" "$test_lineno" "$test_case_name"
            fi
        fi
        echo
    fi

    # check Valgrind results
    if [ -f "$valgrind_file" ]; then
        chmod a+r "$valgrind_file"
        printf '%% Valgrind log:\n'
        cat "$valgrind_file"
        echo
        if grep -q 'valgrind: fatal error:' "$valgrind_file"; then
            # There was an error in Valgrind. Treat this test case as skipped.
            log_stdout SKIPPED
            echo
            return
        fi
        if grep 'ERROR SUMMARY:' "$valgrind_file" | grep -qv ' 0 errors'; then
            failed="true"
            eprintf '%s:%d: %s: Valgrind detected error\n' \
                "$test_file" "$test_lineno" "$test_case_name"
        fi
    fi

    if "$should_succeed"; then
        if "$failed"; then
            log_stdout 'ERROR[FAILED]'
        else
            log_stdout 'OK[PASSED]'
        fi
    else
        if "$failed"; then
            log_stdout 'OK[FAILED_AS_EXPECTED]'
        else
            log_stdout 'ERROR[PASSED_UNEXPECTEDLY]'
            eprintf '%s:%d: %s: passed unexpectedly\n' \
                "$test_file" "$test_lineno" "$test_case_name"
        fi
    fi
    echo
}

alias test_x='testcase "$LINENO" 3<<\__IN__ 4<&- 5<&-'
alias test_o='testcase "$LINENO" 3<<\__IN__ 4<<\__OUT__ 5<&-'
alias test_O='testcase "$LINENO" 3<<\__IN__ 4</dev/null 5<&-'
alias test_e='testcase "$LINENO" 3<<\__IN__ 4<&- 5<<\__ERR__'
alias test_oe='testcase "$LINENO" 3<<\__IN__ 4<<\__OUT__ 5<<\__ERR__'
alias test_Oe='testcase "$LINENO" 3<<\__IN__ 4</dev/null 5<<\__ERR__'
alias test_E='testcase "$LINENO" 3<<\__IN__ 4<&- 5</dev/null'
alias test_oE='testcase "$LINENO" 3<<\__IN__ 4<<\__OUT__ 5</dev/null'
alias test_OE='testcase "$LINENO" 3<<\__IN__ 4</dev/null 5</dev/null'

##### Run test

# The test is run in a subshell. The main shell waits for a possible trap and
# lastly removes the temporary directory.
(
abs_test_file="$(absolute "$test_file")"
abs_suppressions="$(absolute valgrind.supp)"
cd "$work_dir"

ln -s -- "$testee" sh
testee_sh="$(absolute sh)"
export TESTEE="$testee"

. "$abs_test_file"
)

# vim: set ts=8 sts=4 sw=4 et:
