# until-y.tst: yash-specific test of until loop

test_OE 'effect of empty condition (true)'
true
until do echo not reached; break; done
__IN__

test_OE 'effect of empty condition (false)'
false
until do echo not reached; break; done
__IN__

test_oE 'effect of empty body'
i=0
until [ $((i=i+1)) -gt 2 ];do done
echo $i
__IN__
3
__OUT__

test_OE -e 0 'exit status with empty condition'
false
until do echo not reached; break; done
__IN__

test_OE -e 0 'exit status with empty body (0-round loop)'
i=0
until [ $((i=i+1)) -gt 0 ];do done
__IN__

test_OE -e 0 'exit status with empty body (1-round loop)'
i=0
until [ $((i=i+1)) -gt 1 ];do done
__IN__

test_OE -e 0 'exit status with empty body (2-round loop)'
i=0
until [ $((i=i+1)) -gt 2 ];do done
__IN__

(
posix="true"

test_Oe -e 2 'POSIX: empty condition (single line)'
until do echo not reached; done
__IN__
syntax error: commands are missing after `until'
__ERR__
#'
#`

test_Oe -e 2 'POSIX: empty condition (multi-line)'
until
do echo not reached; done
__IN__
syntax error: commands are missing after `until'
__ERR__
#'
#`

test_Oe -e 2 'POSIX: empty body (single line)'
until echo not reached; do done
__IN__
syntax error: commands are missing between `do' and `done'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (multi-line)'
until echo not reached; do
done
__IN__
syntax error: commands are missing between `do' and `done'
__ERR__
#'
#`
#'
#`

)

test_Oe -e 2 'missing do'
until echo not reached; done
__IN__
syntax error: encountered `done' without a matching `do'
syntax error: (maybe you missed `do'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'missing do and done'
until echo not reached
__IN__
syntax error: `do' is missing
syntax error: `done' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'missing done'
until echo not reached 1; do echo not reached 2
__IN__
syntax error: `done' is missing
__ERR__
#'
#`

test_Oe -e 2 'missing do and done (in grouping)'
{ until }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `do'?)
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'missing done (in grouping)'
{ until do }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`

# `do' and `done' without `until` are tested in for-y.tst.

# vim: set ft=sh ts=8 sts=4 sw=4 et:
