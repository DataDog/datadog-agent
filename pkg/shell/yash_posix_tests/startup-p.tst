# startup-p.tst: test of shell startup for any POSIX-compliant shell

test_O -e 17 'one operand with -c' -c 'exit 17'
__IN__

test_o -e 0 'two operands with -c' \
    -c 'printf "[%s]\n" "$0" "$@"' 'command  name'
__IN__
[command  name]
__OUT__

test_o -e 0 'one positional parameter with -c' \
    -c 'printf "[%s]\n" "$0" "$@"' 0 1
__IN__
[0]
[1]
__OUT__

test_o -e 0 'many positional parameters with -c' \
    -c 'printf "[%s]\n" "$0" "$@"' 0 1 '2  2' 3 4 - 6 7 8 9 10 11
__IN__
[0]
[1]
[2  2]
[3]
[4]
[-]
[6]
[7]
[8]
[9]
[10]
[11]
__OUT__

test_oE -e 0 'stdin is not used with -c' -c 'cat'
printed text
__IN__
printed text
__OUT__

test_oE -e 19 'no operands with -s' -s
echo $#
exit 19
__IN__
0
__OUT__

test_oE -e 23 'one operand with -s' -s '1  1'
printf "[%s]\n" "$@"
exit 23
__IN__
[1  1]
__OUT__

test_oE 'two operands with -s' -s '1  1' 2
printf "[%s]\n" "$@"
__IN__
[1  1]
[2]
__OUT__

test_oE 'many operands with -s' -s '1  1' 2 3 4 - 6 7 8 9 10 11
printf "[%s]\n" "$@"
__IN__
[1  1]
[2]
[3]
[4]
[-]
[6]
[7]
[8]
[9]
[10]
[11]
__OUT__

testcase "$LINENO" '$0 with -s' -s X 3<<\__IN__ 4<<__OUT__ 5<&-
printf '[%s]\n' "$0"
__IN__
[$TESTEE]
__OUT__

(
input=./input$LINENO
cat >"$input" <<\__END__
echo input "$*"
cat
exit 3
echo not reached
__END__

test_oE -e 3 'reading file w/o positional parameters' "$input"
stdin
__IN__
input 
stdin
__OUT__

test_oE -e 3 'reading file with one positional parameter' "$input" '1  1'
stdin
__IN__
input 1  1
stdin
__OUT__

test_oE -e 3 'reading file with many positional parameters' \
    "$input" '1  1' 2 3 4 - 6 7 8 9 10 11
stdin
__IN__
input 1  1 2 3 4 - 6 7 8 9 10 11
stdin
__OUT__

)

test_O -d -e 127 'reading non-existing file' ./_no_such_file_
__IN__

(
input=input$LINENO
>"$input"
chmod a-r "$input"

# Skip if we're root.
if { <"$input"; } 2>/dev/null; then
    skip="true"
fi

test_O -d -e n 'reading non-readable file' "$input"
__IN__

)

test_o -d 'all short options' -abCefsuvx +mn
case $- in (*a*)
    case $- in (*b*)
        case $- in (*C*)
            case $- in (*e*)
                case $- in (*f*)
                    case $- in (*u*)
                        case $- in (*v*)
                            case $- in (*x*)
                                echo OK
                            esac
                        esac
                    esac
                esac
            esac
        esac
    esac
esac
__IN__
OK
__OUT__

test_oE 'first operand is ignored if it is a hyphen (-c)' -c - 'echo OK'
__IN__
OK
__OUT__

test_oE 'first operand is ignored if it is a hyphen (no -c or -s)' -
echo OK
__IN__
OK
__OUT__

test_oE 'first operand is ignored if it is a double-hyphen (-c)' -c -- 'echo OK'
__IN__
OK
__OUT__

test_oE 'first operand is ignored if it is a double-hyphen (no -c or -s)' --
echo OK
__IN__
OK
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
