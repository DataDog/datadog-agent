# return-y.tst: yash-specific test of the return built-in

test_OE -e 13 'returning from shell, non-interactive'
return 13
echo not reached
__IN__

test_o -d 'returning from shell, interactive' -i +m
return 13
echo continued $?
__IN__
continued 1
__OUT__

test_O -e 7 'returning from function, interactive' -i +m
func() { return 7; echo not reached; }
func
__IN__

#####

cat <<\__END__ >return
echo in return
return
echo out return, not reached
__END__

cat <<\__END__ >noreturn
echo noreturn
__END__

(
posix="true"
export ENV="$PWD/return"
test_o 'returning from initialization script ($ENV)' -i +m
__IN__
in return
__OUT__
)

test_o 'returning from profile script' \
    -il +m --profile="$PWD/return" --rcfile="$PWD/noreturn"
__IN__
in return
noreturn
__OUT__

test_o 'returning from rcfile script' \
    -il +m --profile="$PWD/noreturn" --rcfile="$PWD/return"
__IN__
noreturn
in return
__OUT__

test_o 'returning from dot script, interactive' -i +m
. ./return
. ./noreturn
__IN__
in return
noreturn
__OUT__

#####

# This test depends on the fact that yash handles USR1 before USR2
# when the two signals are caught while executing a single command.
test_oE 'returning from trap'
trap 'echo USR1; return 13; echo not reached in trap' USR1
trap 'echo USR2' USR2
k() {
    (kill -s USR1 $$; kill -s USR2 $$)
    echo done
}
k
__IN__
USR1
USR2
done
__OUT__

#####

test_OE -e 7 'returning out of eval (iteration)'
fn() {
    eval -i 'return 7' 'echo not reached 1'
    echo not reached 2
}
fn
__IN__

test_O -e 127 'returning from auxiliary (iteration)'
COMMAND_NOT_FOUND_HANDLER=('return 1' 'echo not reached $?')
./_no_such_command_
__IN__

#####

test_oE 'using -n option'
return -n 7
echo $?
return -n 11
echo $?
return --no-return 11
echo $?
__IN__
7
11
11
__OUT__

test_oE -- 'not returning from function with -n'
fn() {
    return -n 7
    echo $?
}
fn
__IN__
7
__OUT__

#####

test_Oe -e n 'too many operands'
return 1 2
__IN__
return: too many operands are specified
__ERR__

test_Oe -e n 'invalid operand: not a integer'
return x
echo not reached
__IN__
return: `x' is not a valid integer
__ERR__
#'
#`

test_Oe -e n 'invalid operand: negative integer'
return -- -100
echo not reached
__IN__
return: `-100' is not a valid integer
__ERR__
#'
#`

test_Oe -e n 'invalid operand: too large integer'
return 999999999999999999999999999999999999999999999999999999999999999999999999
__IN__
return: `999999999999999999999999999999999999999999999999999999999999999999999999' is not a valid integer
__ERR__
#'
#`

test_Oe -e n 'invalid option'
return --no-such-option ''
__IN__
return: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
