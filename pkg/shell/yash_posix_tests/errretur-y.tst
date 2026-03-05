# errretur-y.tst: yash-specific test of the errreturn option

# Many test cases in this file use the "! ( ... )" pattern because it would be
# otherwise impossible to distinguish the behavior of errreturn and errexit.

# An expansion error in a non-interactive shell causes immediate exit of the
# shell (regardless of errexit), so expansion errors should be tested in an
# interactive shell.

test_o -e 0 'errreturn: successful simple command' -o errreturn
true
echo reached
__IN__
reached
__OUT__

test_O -e n 'errreturn: failed simple command outside function' -o errreturn
false
echo not reached
__IN__

test_O -e n 'errreturn: failed simple command inside function' -o errreturn
f() { false; echo not reached 1; }
f
echo not reached 2
__IN__

test_o 'errreturn: negated function call' -o errreturn
f() { false; echo not reached; }
! f
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: negated function call' -e
f() { false; echo reached; }
! f
__IN__
reached
__OUT__

test_o 'errreturn: negated subshell' -o errreturn
! ( false; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: negated subshell' -e
! ( false; echo reached; )
__IN__
reached
__OUT__

cat > dot1 <<__END__
false
echo echo
__END__

test_o 'errreturn: negated dot builtin call' -o errreturn
! . ./dot1
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: negated dot builtin call' -e
! . ./dot1
__IN__
echo
__OUT__

test_o 'errreturn: independent redirection error' -o errreturn
! ( <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: independent redirection error' -e
! ( <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on simple command' -o errreturn
! ( echo not printed <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on simple command' -e
! ( echo not printed <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on subshell' -o errreturn
! ( ( :; ) <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on subshell' -e
! ( ( :; ) <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on grouping' -o errreturn
! ( { :; } <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on grouping' -e
! ( { :; } <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on for loop' -o errreturn
! ( for i in i; do :; done <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on for loop' -e
! ( for i in i; do :; done <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on case' -o errreturn
! ( case i in esac <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on case' -e
! ( case i in esac <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on if' -o errreturn
! ( if :; then :; fi <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on if' -e
! ( if :; then :; fi <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on while loop' -o errreturn
! (
while echo not reached 1; false; do :; done <_no_such_file_
echo not reached 2
)
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on while loop' -e
! (
while echo not reached; false; do :; done <_no_such_file_
echo reached
)
__IN__
reached
__OUT__

test_o 'errreturn: redirection error on until loop' -o errreturn
! ( until echo not reached; do :; done <_no_such_file_; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: redirection error on until loop' -e
! ( until echo not reached; do :; done <_no_such_file_; echo reached; )
__IN__
reached
__OUT__

test_o -e 0 'errreturn: middle of pipeline' -o errreturn
false | false | true
echo reached
__IN__
reached
__OUT__

test_o 'errreturn: last of pipeline' -o errreturn
! ( true | true | false; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: last of pipeline' -e
! ( true | true | false; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: negated pipeline' -o errreturn
! false | false | true
! true | true | false
echo reached $?
__IN__
reached 0
__OUT__

test_o -e 0 'errreturn: initially failing and list' -o errreturn
false && ! echo not reached && ! echo not reached
echo reached
__IN__
reached
__OUT__

test_o 'errreturn: finally failing and list' -o errreturn
! ( true && true && false; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: finally failing and list' -e
! ( true && true && false; echo reached; )
__IN__
reached
__OUT__

test_o -e 0 'errreturn: all succeeding and list' -o errreturn
true && true && true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errreturn: initially succeeding or list' -o errreturn
true || echo not reached || echo not reached
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errreturn: finally succeeding or list' -o errreturn
false || false || true
echo reached
__IN__
reached
__OUT__

test_o 'errreturn: all failing or list' -o errreturn
! ( false || false || false; echo not reached; )
echo reached $?
__IN__
reached 0
__OUT__

test_o -e n 'errexit: all failing or list' -e
! ( false || false || false; echo reached; )
__IN__
reached
__OUT__

test_o 'errreturn: subshell' -o errreturn
! (
    (echo reached 1; false; echo not reached 2) | cat
    (echo reached 3; false; echo not reached 4)
    echo not reached 5
)
echo reached 6 $?
__IN__
reached 1
reached 3
reached 6 0
__OUT__

test_o -e n 'errexit: subshell' -e
! (
    (echo reached 1; false; echo reached 2) | cat
    (echo reached 3; false; echo reached 4)
    echo reached 5
)
__IN__
reached 1
reached 2
reached 3
reached 4
reached 5
__OUT__

test_o 'errreturn: grouping' -o errreturn
! (
    { echo reached 1; false; echo not reached 2; } | cat
    { echo reached 3; false; echo not reached 4; }
    echo not reached 5
)
echo reached 6 $?
__IN__
reached 1
reached 3
reached 6 0
__OUT__

test_o -e n 'errexit: grouping' -e
! (
    { echo reached 1; false; echo reached 2; } | cat
    { echo reached 3; false; echo reached 4; }
    echo reached 5
)
__IN__
reached 1
reached 2
reached 3
reached 4
reached 5
__OUT__

test_o 'errreturn: expansion error in for word' -i +m -o errreturn
f() { for i in ${a?}; do echo not reached 1; done; echo not reached 2; }
! f
echo reached $?
__IN__
reached 0
__OUT__

test_o 'errexit: expansion error in for word' -i +m -e
f() { for i in ${a?}; do echo not reached 1; done; echo not reached 2; }
! f
echo reached $?
__IN__
reached 2
__OUT__

test_o 'errreturn: for loop body' -o errreturn
! (
    for i in 1 2 3; do
        echo a $i
        test $i -ne 2
        echo b $i
    done
    echo not reached
)
echo reached $?
__IN__
a 1
b 1
a 2
reached 0
__OUT__

test_o -e n 'errexit: for loop body' -e
! (
    for i in 1 2 3; do
        echo a $i
        test $i -ne 2
        echo b $i
    done
    echo reached
)
__IN__
a 1
b 1
a 2
b 2
a 3
b 3
reached
__OUT__

test_o 'errreturn: expansion error in case word' -i +m -o errreturn
f() { case ${a?} in (*) esac; echo not reached; }
! f
echo reached $?
__IN__
reached 0
__OUT__

test_o 'errexit: expansion error in case word' -i +m -e
f() { case ${a?} in (*) esac; echo not reached; }
! f
echo reached $?
__IN__
reached 2
__OUT__

test_o 'errreturn: expansion error in case pattern' -i +m -o errreturn
f() { case a in (${a?}) esac; echo not reached; }
! f
echo reached $?
__IN__
reached 0
__OUT__

test_o 'errexit: expansion error in case pattern' -i +m -e
f() { case a in (${a?}) esac; echo not reached; }
! f
echo reached $?
__IN__
reached 2
__OUT__

test_o 'errreturn: case body' -o errreturn
! (
    case a in (a)
        echo reached 1
        false
        echo not reached 2
    esac
    echo not reached 3
)
echo reached 4 $?
__IN__
reached 1
reached 4 0
__OUT__

test_o -e n 'errexit: case body' -e
! (
    case a in (a)
        echo reached 1
        false
        echo reached 2
    esac
    echo reached 3
)
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e 0 'errreturn: if condition' -o errreturn
if false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o -e 0 'errreturn: function call inside if condition' -o errreturn
f() { false; echo not reached $1; }
if f 1; then echo failed 1; else echo passed 1; fi
if f 2 || echo passed 2; then :; fi
__IN__
passed 1
passed 2
__OUT__

test_o -e 0 'errexit: function call inside if condition' -e
f() { false; true; }
if f 1; then echo passed 1; else echo failed 1; fi
if f 2 || echo failed 2; then echo passed 2; fi
__IN__
passed 1
passed 2
__OUT__

test_o -e 0 'errreturn: subshell inside if condition' -o errreturn
if (false; echo not reached 1); then echo failed 1; else echo passed 1; fi
if (false; echo not reached 2) || echo passed 2; then :; fi
__IN__
passed 1
passed 2
__OUT__

test_o -e 0 'errexit: subshell inside if condition' -e
if (false; true); then echo passed 1; else echo failed 1; fi
if (false; true) || echo failed 2; then echo passed 2; fi
__IN__
passed 1
passed 2
__OUT__

test_o -e 0 'errreturn: elif condition' -o errreturn
if false; then
    :
elif false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o 'errreturn: then body' -o errreturn
! (
    if true; then echo reached 1; false; echo not reached 2; fi
    echo not reached 3
)
echo reached 4 $?
__IN__
reached 1
reached 4 0
__OUT__

test_o -e n 'errexit: then body' -e
! (
    if true; then echo reached 1; false; echo reached 2; fi
    echo reached 3
)
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o 'errreturn: else body' -o errreturn
! (
    if false; then :; else echo reached 1; false; echo not reached 2; fi
    echo not reached 3
)
echo reached 4 $?
__IN__
reached 1
reached 4 0
__OUT__

test_o -e n 'errexit: else body' -e
! (
    if false; then :; else echo reached 1; false; echo reached 2; fi
    echo reached 3
)
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e 0 'errreturn: while condition' -o errreturn
while false; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o 'errreturn: while body' -o errreturn
! (
    while true; do
        echo reached 1
        false
        echo not reached 2
        break
    done
    echo not reached 3
)
echo reached 4 $?
__IN__
reached 1
reached 4 0
__OUT__

test_o -e n 'errexit: while body' -e
! (
    while true; do
        echo reached 1
        false
        echo reached 2
        break
    done
    echo reached 3
)
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e 0 'errreturn: until condition' -o errreturn
until false; true; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o 'errreturn: until body' -o errreturn
! (
    until false; do
        echo reached 1
        false
        echo not reached 2
        break
    done
    echo not reached 3
)
echo reached 4 $?
__IN__
reached 1
reached 4 0
__OUT__

test_o -e n 'errexit: until body' -e
! (
    until false; do
        echo reached 1
        false
        echo reached 2
        break
    done
    echo reached 3
)
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o 'no return in interactive shell' -i +m -o errreturn
false
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit supersedes errreturn' -i +m -e -o errreturn
false
echo not reached
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
