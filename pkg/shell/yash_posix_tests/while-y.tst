# while-y.tst: yash-specific test of while loop

test_oE 'effect of empty condition (true)'
true
while do echo -; break; done
__IN__
-
__OUT__

test_oE 'effect of empty condition (false)'
false
while do echo -; break; done
__IN__
-
__OUT__

test_oE 'effect of empty body'
i=0
while [ $((i=i+1)) -le 2 ];do done
echo $i
__IN__
3
__OUT__

test_OE -e 0 'exit status with empty condition'
while do break; done
__IN__

test_OE -e 0 'exit status with empty body (0-round loop)'
i=0
while [ $((i=i+1)) -le 0 ];do done
__IN__

test_OE -e 0 'exit status with empty body (1-round loop)'
i=0
while [ $((i=i+1)) -le 1 ];do done
__IN__

test_OE -e 0 'exit status with empty body (2-round loop)'
i=0
while [ $((i=i+1)) -le 2 ];do done
__IN__

(
posix="true"

test_Oe -e 2 'POSIX: empty condition (single line)'
while do echo not reached; done
__IN__
syntax error: commands are missing after `while'
__ERR__
#'
#`

test_Oe -e 2 'POSIX: empty condition (multi-line)'
while
do echo not reached; done
__IN__
syntax error: commands are missing after `while'
__ERR__
#'
#`

test_Oe -e 2 'POSIX: empty body (single line)'
while echo not reached; do done
__IN__
syntax error: commands are missing between `do' and `done'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (multi-line)'
while echo not reached; do
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
while echo not reached; done
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
while echo not reached
__IN__
syntax error: `do' is missing
syntax error: `done' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'missing done'
while echo not reached 1; do echo not reached 2
__IN__
syntax error: `done' is missing
__ERR__
#'
#`

test_Oe -e 2 'missing do and done (in grouping)'
{ while }
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
{ while do }
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

# `do' and `done' without `while` are tested in for-y.tst.

# vim: set ft=sh ts=8 sts=4 sw=4 et:
