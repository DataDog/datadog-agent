# getopts-y.tst: yash-specific test of the getopts built-in

test_E 'no error message on missing option argument (with :)'
getopts :a: o -a
__IN__

test_o '":" is not parsed as valid option'
getopts : o -:
echo "$?" "$o" "$OPTARG"
__IN__
0 ? :
__OUT__

(
posix="true"

test_Oe -e 2 'invalid option candidate "?"'
getopts '?' o
__IN__
getopts: `?' is not a valid option specification
__ERR__
#'
#`

test_Oe -e 2 'invalid option candidate ":"'
getopts :: o
__IN__
getopts: `::' is not a valid option specification
__ERR__
#'
#`

test_Oe -e 2 'invalid option candidate "-"'
getopts - o
__IN__
getopts: `-' is not a valid option specification
__ERR__
#'
#`

test_Oe -e 2 'invalid option candidate "+"'
getopts + o
__IN__
getopts: `+' is not a valid option specification
__ERR__
#'
#`

)

test_Oe -e 2 'invalid operand variable name'
getopts '' =
__IN__
getopts: `=' is not a valid variable name
__ERR__
#'
#`

test_Oe -e 2 'unset OPTIND'
unset OPTIND
getopts a o -a
__IN__
getopts: $OPTIND has an invalid value
__ERR__

test_Oe -e 2 'empty OPTIND'
OPTIND=
getopts a o -a
__IN__
getopts: $OPTIND has an invalid value
__ERR__

test_Oe -e 2 'non-numeric OPTIND'
OPTIND=X
getopts a o -a
__IN__
getopts: $OPTIND has an invalid value
__ERR__

test_oE 'OPTIND argument index out-of-range'
OPTIND=100
getopts a o -a
echo "$?" "$o" "$OPTIND"
__IN__
1 ? 100
__OUT__

test_oE 'OPTIND option index out-of-range'
OPTIND=1:10
getopts abc o -abc
echo "$?" "$o" "$OPTIND"
__IN__
1 ? 2
__OUT__

test_oE 'getopts has no effect after all options have been parsed'
getopts a o -a
getopts a o -a
echo "$?" "$o" "$OPTIND"
getopts a o -a
echo "$?" "$o" "$OPTIND"
__IN__
1 ? 2
1 ? 2
__OUT__

test_Oe -e 2 'read-only operand variable'
readonly o
getopts a o -a
__IN__
getopts: $o is read-only
__ERR__

test_Oe -e 2 'read-only OPTARG'
readonly OPTARG
getopts a: o -a foo
__IN__
getopts: $OPTARG is read-only
__ERR__

test_Oe -e 2 'read-only OPTIND'
readonly OPTIND
getopts a o -a
__IN__
getopts: $OPTIND is read-only
__ERR__

test_Oe -e 2 'invalid option'
getopts --no-such-option a o -a
__IN__
getopts: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'missing operand (0)'
getopts
__IN__
getopts: this command requires 2 operands
__ERR__

test_Oe -e 2 'missing operand (1)'
getopts a
__IN__
getopts: this command requires 2 operands
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
