# test-p.tst: test of the test built-in for any POSIX-compliant shell

posix="true"

# This file is for testing the shell built-in, so we should skip the tests
# if the shell does not seem to implement the test built-in.
case $("$TESTEE" -c 'command -V test') in
    (*built-in*|*builtin*)
        # Okay, the testee seems to support the test command as a built-in.
        # This check is not POSIXly portable because the output of "command -V"
        # is unspecified in POSIX, but it works for yash and most other shells.
        ;;
    (*)
        skip=true
        ;;
esac

umask u=rwx,go=

>file
(umask a-r && >unreadable)
(umask a-w && >unwritable)
(umask a-x && >unexecutable)
>executable
chmod u+x executable

echo >oneline
echo foo >nonempty

mkdir dir setgroupid setuserid
chmod ug-s dir
chmod g+s setgroupid
chmod u+s setuserid

mkfifo fifo

ln file hardlink
ln -s file filelink
ln -s _no_such_file_ brokenlink
ln -s unreadable unreadablelink
ln -s unwritable unwritablelink
ln -s unexecutable unexecutableln
ln -s executable executablelink
ln -s oneline onelinelink
ln -s nonempty nonemptylink
ln -s dir dirlink
ln -s setgroupid setgroupidlink
ln -s setuserid setuseridlink
ln -s fifo fifolink

touch -t 200001010000 older
touch -t 200101010000 newer
touch -a -t 200101010000 old; touch -m -t 200001010000 old
touch -a -t 200001010000 new; touch -m -t 200101010000 new

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

assert_false

assert_false ''
assert_true -
assert_true --
assert_true A
assert_true X
assert_true ABC
assert_true xyz

(
block_file="$(find /dev -type b 2>/dev/null | head -n 1)"
if [ -e "$block_file" ]; then
    ln -s "$block_file" blocklink
else
    skip="true"
fi

assert_true -b
assert_true -b "$block_file"
assert_true -b blocklink
assert_false -b dir
assert_false -b dirlink
assert_false -b ./_no_such_file_
assert_false -b brokenlink
)

(
character_file="$(find /dev/tty /dev -type c 2>/dev/null | head -n 1)"
if [ -e "$character_file" ]; then
    ln -s "$character_file" characterlink
else
    skip="true"
fi

assert_true -c
assert_true -c "$character_file"
assert_true -c characterlink
assert_false -c dir
assert_false -c dirlink
assert_false -c ./_no_such_file_
assert_false -c brokenlink
)

assert_true -d
assert_true -d .
assert_true -d ..
assert_true -d dir
assert_true -d /dev
assert_true -d dirlink
assert_false -d file
assert_false -d filelink
assert_false -d ./_no_such_file_
assert_false -d brokenlink

assert_true -e
assert_true -e .
assert_true -e ..
assert_true -e dir
assert_true -e dirlink
assert_true -e file
assert_true -e filelink
assert_true -e /dev/null
assert_false -e ./_no_such_file_
assert_false -e brokenlink

assert_true -f
assert_true -f file
assert_true -f filelink
assert_false -f dir
assert_false -f dirlink
assert_false -f ./_no_such_file_
assert_false -f brokenlink

assert_true -g
assert_true -g setgroupid
assert_true -g setgroupidlink
assert_false -g dir
assert_false -g dirlink
assert_false -g file
assert_false -g filelink
assert_false -g ./_no_such_file_
assert_false -g brokenlink

assert_true -h
assert_true -h filelink
assert_true -h brokenlink
assert_false -h dir
assert_false -h file
assert_false -h ./_no_such_file_

assert_true -L
assert_true -L filelink
assert_true -L brokenlink
assert_false -L dir
assert_false -L file
assert_false -L ./_no_such_file_

assert_true -n
assert_false -n ''
assert_true -n .
assert_true -n ..
assert_true -n ...
assert_true -n A
assert_true -n xyz

assert_true -p
assert_true -p fifo
assert_true -p fifolink
assert_false -p dir
assert_false -p dirlink
assert_false -p file
assert_false -p filelink
assert_false -p ./_no_such_file_
assert_false -p brokenlink

assert_true -r
assert_true -r file
assert_true -r filelink
(
if [ -r unreadable ]; then
    skip="true"
fi
assert_false -r unreadable
)
(
if [ -r unreadablelink ]; then
    skip="true"
fi
assert_false -r unreadablelink
)
assert_true -r dir
assert_true -r dirlink
assert_false -r ./_no_such_file_
assert_false -r brokenlink

assert_true -S
# Tests for the -S operator is missing
assert_false -S file
assert_false -S filelink
assert_false -S dir
assert_false -S dirlink
assert_false -S ./_no_such_file_
assert_false -S brokenlink

assert_true -s
assert_true -s oneline
assert_true -s onelinelink
assert_true -s nonempty
assert_true -s nonemptylink
assert_false -s file
assert_false -s filelink
assert_false -s ./_no_such_file_
assert_false -s brokenlink

assert_true -t
# Other tests for the -t operator are in testtty-p.tst.

assert_true -u
assert_true -u setuserid
assert_true -u setuseridlink
assert_false -u file
assert_false -u filelink
assert_false -u dir
assert_false -u dirlink
assert_false -u ./_no_such_file_
assert_false -u brokenlink

assert_true -w
assert_true -w file
assert_true -w filelink
(
if [ -w unwritable ]; then
    skip="true"
fi
assert_false -w unwritable
)
(
if [ -w unwritablelink ]; then
    skip="true"
fi
assert_false -w unwritablelink
)
assert_true -w dir
assert_true -w dirlink
assert_false -w ./_no_such_file_
assert_false -w brokenlink

assert_true -x
assert_true -x executable
assert_true -x executablelink
(
if [ -x unexecutable ]; then
    skip="true"
fi
assert_false -x unexecutable
)
(
if [ -x unexecutableln ]; then
    skip="true"
fi
assert_false -x unexecutableln
)
assert_true -x dir
assert_true -x dirlink
assert_false -x ./_no_such_file_
assert_false -x brokenlink

assert_true -z
assert_true -z ''
assert_false -z .
assert_false -z ..
assert_false -z ...
assert_false -z A
assert_false -z xyz

assert_true "" = ""
assert_true 1 = 1
assert_true abcde = abcde
assert_false 0 = 1
assert_false abcde = 12345
assert_true ! = !
assert_true = = =
assert_false "(" = ")"

assert_false "" != ""
assert_false 1 != 1
assert_false abcde != abcde
assert_true 0 != 1
assert_true abcde != 12345
assert_false ! != !
assert_false != != !=
assert_true "(" != ")"

assert_true -3 -eq -3
assert_true 90 -eq 90
assert_true 0 -eq 0
assert_false -3 -eq 90
assert_false -3 -eq 0
assert_false 90 -eq 0

assert_false -3 -ne -3
assert_false 90 -ne 90
assert_false 0 -ne 0
assert_true -3 -ne 90
assert_true -3 -ne 0
assert_true 90 -ne 0

assert_false -3 -gt -3
assert_false -3 -gt 0
assert_false 0 -gt 90
assert_true 0 -gt -3
assert_true 90 -gt -3
assert_false 0 -gt 0

assert_true -3 -ge -3
assert_false -3 -ge 0
assert_false 0 -ge 90
assert_true 0 -ge -3
assert_true 90 -ge -3
assert_true 0 -ge 0

assert_false -3 -lt -3
assert_true -3 -lt 0
assert_true 0 -lt 90
assert_false 0 -lt -3
assert_false 90 -lt -3
assert_false 0 -lt 0

assert_true -3 -le -3
assert_true -3 -le 0
assert_true 0 -le 90
assert_false 0 -le -3
assert_false 90 -le -3
assert_true 0 -le 0

# The behavior of the < and > operators cannot be fully tested.
assert_false 11 '<' 100
assert_false 11 '<' 11
assert_true 100 '<' 11

assert_true 11 '>' 100
assert_false 11 '>' 11
assert_false 100 '>' 11

assert_true XXXXX -ot newer
assert_false XXXXX -ot XXXXX
assert_false newer -ot XXXXX
assert_true older -ot newer
assert_false newer -ot newer
assert_false newer -ot older

assert_false XXXXX -nt newer
assert_false XXXXX -nt XXXXX
assert_true newer -nt XXXXX
assert_false older -nt newer
assert_false older -nt older
assert_true newer -nt older

assert_false XXXXX -ef newer
assert_false XXXXX -ef XXXXX
assert_false newer -ef XXXXX
assert_false older -ef newer
assert_true older -ef older
assert_false newer -ef older
assert_true file -ef hardlink
assert_false file -ef newer

assert_true !
assert_true ! ''
assert_false ! A
assert_false ! X
assert_false ! ABC
assert_false ! xyz
assert_false ! -f file
assert_true ! -f dir
assert_true ! -d file
assert_false ! -d dir
assert_true ! -n ''
assert_false ! -n .
assert_false ! a = a # ! ( a = a )
assert_false ! ! -n ""
assert_true ! ! -n 1
assert_true ! ! ! ""
assert_false ! ! ! 1

# vim: set ft=sh ts=8 sts=4 sw=4 et:
