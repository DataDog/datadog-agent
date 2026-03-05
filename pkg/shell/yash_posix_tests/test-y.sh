# test-y.sh: utility for testing the test built-in

# $1 = $LINENO, $2 = expected exit status, $3... = expression
assert() (
    setup <<\__END__
    test "$@"
    result_test=$?
    [ "$@" ]
    result_bracket=$?
    case "$result_test" in ("$result_bracket")
        exit "$result_bracket"
    esac
    printf 'result_test=%d result_bracket=%d\n' "$result_test" "$result_bracket"
    exit 100
__END__

    lineno="$1"
    expected_exit_status="$2"
    shift 2
    testcase "$lineno" -e "$expected_exit_status" "test $*" -s -- "$@" \
        3</dev/null 4<&3 5<&3
)

alias assert_true='assert "$LINENO" 0'
alias assert_false='assert "$LINENO" 1'

# vim: set ft=sh ts=8 sts=4 sw=4 et:
