# getopts-p.tst: test of the getopts built-in for any POSIX-compliant shell

posix="true"

test_o 'default OPTIND is 1'
printf '%s\n' "$OPTIND"
__IN__
1
__OUT__

test_o 'OPTIND and OPTARG are not exported by default'
getopts a: o -a arg
getopts a: o -a arg
sh -c 'echo ${OPTIND-unset} ${OPTARG-unset}'
__IN__
1 unset
__OUT__

test_o 'operand variable is updated to parsed option on each invocation'
getopts ab:c o -a -b arg -c
printf '1[%s]\n' "$o"
getopts ab:c o -a -b arg -c
printf '2[%s]\n' "$o"
getopts ab:c o -a -b arg -c
printf '3[%s]\n' "$o"
__IN__
1[a]
2[b]
3[c]
__OUT__

test_x -e 0 'exit status is zero after option is parsed' -e
getopts ab:c o -a -b arg -c
getopts ab:c o -a -b arg -c
getopts ab:c o -a -b arg -c
__IN__

test_x -e 1 'exit status is one after parsing all options' -e
getopts ab:c o -a -b arg -c
getopts ab:c o -a -b arg -c
getopts ab:c o -a -b arg -c
getopts ab:c o -a -b arg -c
__IN__

test_o 'OPTIND is set when option argument is parsed: empty'
getopts a:b o -a '' -b
echo "[$OPTIND]"
__IN__
[3]
__OUT__

test_o 'OPTIND is set when option argument is parsed: non-empty separate'
getopts a:b o -a '-x  foo' -b
echo "[$OPTIND]"
__IN__
[3]
__OUT__

test_o 'OPTIND is set when option argument is parsed: non-empty adjoined'
getopts a:b o -a'  foo' -b
echo "[$OPTIND]"
__IN__
[2]
__OUT__

test_o 'OPTARG is set when option argument is parsed: empty'
getopts a: o -a ''
echo "[$OPTARG]"
__IN__
[]
__OUT__

test_o 'OPTARG is set when option argument is parsed: non-empty separate'
getopts a: o -a '-x  foo'
echo "[$OPTARG]"
__IN__
[-x  foo]
__OUT__

test_o 'OPTARG is set when option argument is parsed: non-empty adjoined'
getopts a: o -a'  foo'
echo "[$OPTARG]"
__IN__
[  foo]
__OUT__

test_o 'OPTARG is unset when option without argument is parsed'
getopts a o -a
echo "${OPTARG-un}${OPTARG-set}"
__IN__
unset
__OUT__

test_o 'operand variable is set to "?" on unknown option'
getopts '' o -a
printf '[%s]\n' "$o"
__IN__
[?]
__OUT__

test_o 'OPTARG is set to the option on unknown option (with :)'
getopts : o -a
printf '[%s]\n' "$OPTARG"
__IN__
[a]
__OUT__

test_E 'no error message on unknown option (with :)'
getopts : o -a
__IN__

test_o 'OPTARG is unset on unknown option (without :)'
getopts '' o -a
printf '%s\n' "${OPTARG-un}${OPTARG-set}"
__IN__
unset
__OUT__

test_x -d 'error message is printed on unknown option (without :)'
getopts '' o -a
__IN__

test_o 'operand variable is set to ":" on missing option argument (with :)'
getopts :a: v -a
printf '[%s]\n' "$v"
__IN__
[:]
__OUT__

test_o 'OPTARG is set to the option on missing option argument (with :)'
getopts :a: v -a
printf '[%s]\n' "$OPTARG"
__IN__
[a]
__OUT__

test_o 'operand variable is set to "?" on missing option argument (without :)'
getopts a: v -a
printf '[%s]\n' "$v"
__IN__
[?]
__OUT__

test_o 'OPTARG is unset on missing option argument (without :)'
getopts a: v -a
printf '%s\n' "${OPTARG-un}${OPTARG-set}"
__IN__
unset
__OUT__

test_x -d 'error message is printed on missing option argument (without :)'
getopts a: v -a
__IN__

test_o 'operand variable is set to "?" after parsing all options'
getopts a x -a
getopts a x -a
printf '[%s]\n' "$x"
__IN__
[?]
__OUT__

test_o 'OPTARG is unset after parsing all options'
getopts a x -a
getopts a x -a
printf '%s\n' "${OPTARG-un}${OPTARG-set}"
__IN__
unset
__OUT__

test_o 'options can be grouped after single hyphen'
getopts abc o -abc
printf '1[%s]\n' "$o"
getopts abc o -abc
printf '2[%s]\n' "$o"
getopts abc o -abc
printf '3[%s]\n' "$o"
getopts abc o -abc
printf '4[%s]\n' "$o"
__IN__
1[a]
2[b]
3[c]
4[?]
__OUT__

test_x -e 1 'single hyphen is not an option but an operand'
getopts '' x -
__IN__

test_o 'double hyphen separates options and operands'
getopts ab x -a -- -b
printf '1[%s]\n' "$x"
getopts ab x -a -- -b ||
printf '2[%d]\n' "$OPTIND"
__IN__
1[a]
2[3]
__OUT__

test_o 'OPTIND is first operand index after parsing all options: no operand, no --'
getopts '' x
printf '%d\n' "$OPTIND"
__IN__
1
__OUT__

test_o 'OPTIND is first operand index after parsing all options: one operand, no --'
getopts '' x operand
printf '%d\n' "$OPTIND"
__IN__
1
__OUT__

test_o 'OPTIND is first operand index after parsing all options: no operand, with --'
getopts '' x --
printf '%d\n' "$OPTIND"
__IN__
2
__OUT__

test_o 'OPTIND is first operand index after parsing all options: one operand, with --'
getopts '' x -- operand
printf '%d\n' "$OPTIND"
__IN__
2
__OUT__

test_o 'resetting OPTIND to parse another arguments'
getopts ab p -a -b
getopts ab p -a -b
getopts ab p -a -b
OPTIND=1
getopts xy q -x -y
printf '1[%s]\n' "$q"
getopts xy q -x -y
printf '2[%s]\n' "$q"
getopts xy q -x -y
printf '3[%d]\n' "$OPTIND"
__IN__
1[x]
2[y]
3[3]
__OUT__

test_o 'positional parameters are parsed by default' -s -- -a -b arg -c
getopts ab:c o
printf '1[%s]\n' "$o"
getopts ab:c o
printf '2[%s]\n' "$o"
getopts ab:c o
printf '3[%s]\n' "$o"
__IN__
1[a]
2[b]
3[c]
__OUT__

test_o 'option characters are alphanumeric'
getopts ab:01: o -a -b arg -1 -2 -0
printf '1[%s]\n' "$o"
getopts ab:01: o -a -b arg -1 -2 -0
printf '2[%s]\n' "$o"
getopts ab:01: o -a -b arg -1 -2 -0
printf '3[%s]\n' "$o"
getopts ab:01: o -a -b arg -1 -2 -0
printf '4[%s]\n' "$o"
__IN__
1[a]
2[b]
3[1]
4[0]
__OUT__

test_O -d -e n 'readonly OPTIND'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The getopts built-in fails.
# - The getopts built-in succeeds ignoring the readonlyness of the variable.
readonly OPTIND && getopts a opt -- x &&
if [ "$OPTIND" = 2 ]; then
    printf 'OPTIND successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

test_O -d -e n 'readonly OPTARG'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The getopts built-in fails.
# - The getopts built-in succeeds ignoring the readonlyness of the variable.
readonly OPTARG && getopts a: opt -a foo &&
if [ "$OPTARG" = foo ]; then
    printf 'OPTARG successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
