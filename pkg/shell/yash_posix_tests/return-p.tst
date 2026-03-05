# return-p.tst: test of the return built-in for any POSIX-compliant shell

posix="true"

macos_kill_workaround

test_oE 'returning from function, unnested'
fn() {
    echo in function
    return
    echo not reached
}
fn
echo after function
__IN__
in function
after function
__OUT__

test_oE 'returning from function, nested in other functions'
first=true
fn1() {
    echo in fn1
    if $first; then
        first=false
    else
        return
    fi
    echo recurring
    fn1
    echo recurred
}
fn2() {
    echo in fn2
    fn1
    echo out fn2
}
fn2
echo after function
__IN__
in fn2
in fn1
recurring
in fn1
recurred
out fn2
after function
__OUT__

cat <<\__END__ >fn
fn() {
    echo in function
    return
    echo out function, not reached
}
fn
echo after function
__END__

test_oE 'returning from function, nested in dot script'
. ./fn
echo after dot
__IN__
in function
after function
after dot
__OUT__

cat <<\__END__ >return
echo in return
return
echo out return, not reached
__END__

test_oE 'returning from dot script, unnested'
. ./return
echo after .
__IN__
in return
after .
__OUT__

cat <<\__END__ >outer
echo in outer
. ./return
echo out outer
__END__

test_oE 'returning from dot script, nested in another dot script'
. ./outer
echo after .
__IN__
in outer
in return
out outer
after .
__OUT__

test_oE 'returning from dot script, nested in function'
fn() {
    echo in function
    . ./return
    echo out function
}
fn
echo after function
__IN__
in function
in return
out function
after function
__OUT__

test_OE -e 13 'default exit status of returning from function'
fn() {
    (exit 13)
    return
}
fn
__IN__

cat <<\__END__ >exitstatus
(exit 17)
return
__END__

test_OE -e 17 'default exit status of returning from dot script'
. ./exitstatus
__IN__

test_OE -e 13 'specifying exit status in returning from function'
fn() {
    (exit 1)
    return 13
}
fn
__IN__

cat <<\__END__ >exitstatus17
(exit 1)
return 17
__END__

test_OE -e 17 'specifying exit status in returning from dot script'
. ./exitstatus17
__IN__

test_oE -e 0 'default exit status in function in trap'
fn() { true; return; }
trap 'fn; echo trapped $?' USR1
(exit 19)
(kill -s USR1 $$; exit 19)
: # null command to ensure the trap to be handled
__IN__
trapped 19
__OUT__

# TODO Yash does not yet support this
test_oE -e 0 -f 'default exit status in trap in function'
trap '(exit 1); return; echo X $?' INT
f() {
    (kill -INT $$; exit 2)
    echo Y $?
}
f
echo Z $?
__IN__
Z 2
__OUT__

test_OE 'returning out of eval'
fn() {
    eval return
    echo not reached
}
fn
__IN__

test_OE 'returning with !'
fn() {
    ! return
    echo not reached
}
fn
__IN__

test_OE 'returning before &&'
fn() {
    return && echo not reached 1
    echo not reached 2 $?
}
fn
__IN__

test_OE 'returning after &&'
fn() {
    true && return
    echo not reached $?
}
fn
__IN__

test_OE 'returning before ||'
fn() {
    return || echo not reached 1
    echo not reached 2 $?
}
fn
__IN__

test_OE 'returning after ||'
fn() {
    false || return
    echo not reached $?
}
fn
__IN__

test_OE 'returning out of brace'
fn() {
    { return; }
    echo not reached
}
fn
__IN__

test_OE 'returning out of if'
fn() {
    if return; then echo not reached then; else echo not reached else; fi
    echo not reached
}
fn
__IN__

test_OE 'returning out of then'
fn() {
    if true; then return; echo not reached then; else echo not reached else; fi
    echo not reached
}
fn
__IN__

test_OE 'returning out of else'
fn() {
    if false; then echo not reached then; else return; echo not reached else; fi
    echo not reached
}
fn
__IN__

test_OE 'returning out of for loop'
fn() {
    for i in 1; do
        return
        echo not reached in loop
    done
    echo not reached after loop
}
fn
__IN__

test_OE 'returning out of while loop'
fn() {
    while true; do
        return
        echo not reached in loop
    done
    echo not reached after loop
}
fn
__IN__

test_OE 'returning out of until loop'
fn() {
    until false; do
        return
        echo not reached in loop
    done
    echo not reached after loop
}
fn
__IN__

test_OE 'returning out of case'
fn() {
    case x in
        x)
            return
            echo not reached in case
    esac
    echo not reached after esac
}
fn
__IN__

test_OE -e 56 'separator preceding operand'
fn() {
    return -- 56
    echo not reached
}
fn
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
