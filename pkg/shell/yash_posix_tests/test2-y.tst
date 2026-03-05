# test2-y.tst: yash-specific test of the test built-in, part 2

if ! testee -c 'command -bv test' >/dev/null; then
    skip="true"
fi

. ../test-y.sh

assert_true "" == ""
assert_true 1 == 1
assert_true abcde == abcde
assert_false 0 == 1
assert_false abcde == 12345
assert_true ! == !
assert_true == == ==
assert_false "(" == ")"

# The behavior of the ===, !==, <, <=, >, >= operators cannot be fully tested.
# The < and > operators are tested in test-p.tst.
assert_true "" === ""
assert_true 1 === 1
assert_true abcde === abcde
assert_false 0 === 1
assert_false abcde === 12345
assert_true ! === !
assert_true === === ===
assert_false "(" === ")"

assert_false "" !== ""
assert_false 1 !== 1
assert_false abcde !== abcde
assert_true 0 !== 1
assert_true abcde !== 12345
assert_false ! !== !
assert_false !== !== !==
assert_true "(" !== ")"

assert_false 11 '<=' 100
assert_true 11 '<=' 11
assert_true 100 '<=' 11

assert_true 11 '>=' 100
assert_true 11 '>=' 11
assert_false 100 '>=' 11

assert_true  abc123xyz     =~ 'c[[:digit:]]*x'
assert_false -axyzxyzaxyz- =~ 'c[[:digit:]]*x'
assert_true  -axyzxyzaxyz- =~ '-(a|xyz)*-'
assert_false abc123xyz     =~ '-(a|xyz)*-'

assert_true "" -veq ""
assert_true 0 -veq 0
assert_false 0 -veq 1
assert_false 1 -veq 0
assert_true 01 -veq 0001
assert_true .%=01 -veq .%=0001
assert_true 0.01.. -veq 0.1..
assert_false 0.01.0 -veq 0.1.

assert_false "" -vne ""
assert_false 0 -vne 0
assert_true 0 -vne 1
assert_true 1 -vne 0

assert_false "" -vgt ""
assert_false 0 -vgt 0
assert_false 0 -vgt 1
assert_true 1 -vgt 0

assert_true "" -vge ""
assert_true 0 -vge 0
assert_false 0 -vge 1
assert_true 1 -vge 0

assert_false "" -vlt ""
assert_false 0 -vlt 0
assert_true 0 -vlt 1
assert_false 1 -vlt 0
assert_false 0.01.0 -vlt 0.1..
assert_false 0.01.0 -vlt 0.1.:

assert_true "" -vle ""
assert_true 0 -vle 0
assert_true 0 -vle 1
assert_false 1 -vle 0
assert_true 02 -vle 0100
assert_true .%=02 -vle .%=0100
assert_false 0.01.0 -vle 0.1.a0
assert_true 1.2.3 -vle 1.3.2
assert_true -2 -vle -3

# vim: set ft=sh ts=8 sts=4 sw=4 et:
