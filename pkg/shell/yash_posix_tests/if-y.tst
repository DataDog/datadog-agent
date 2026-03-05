# if-y.tst: yash-specific test of if conditional construct

test_oE 'effect of empty condition (if, true)'
true
if then echo true; else echo false; fi
__IN__
true
__OUT__

test_oE 'effect of empty condition (if, false)'
false
if then echo true; else echo false; fi
__IN__
true
__OUT__

test_oE 'effect of empty condition (elif, false)'
false
if false; then echo X; elif then echo true; else echo false; fi
__IN__
true
__OUT__

test_E -e 0 'exit status of empty body (if, true)'
false
if true; then fi
__IN__

test_E -e 0 'exit status of empty body (if-else, true)'
false
if true; then else fi
__IN__

test_E -e 17 'exit status of empty body (if-else, false)'
if(exit 17)then else fi
__IN__

test_E -e 0 'exit status of empty body (if-elif, false-true)'
if false; then elif true; then fi
__IN__

test_E -e 19 'exit status of empty body (if-elif-else, false-false)'
if false; then elif(exit 19)then else fi
__IN__

# TODO: this behavior seems contradictory to the results of above tests
test_E -e 23 'exit status of empty condition and body'
(exit 23)
if then elif then else fi
__IN__

(
posix="true"

test_Oe -e 2 'POSIX: empty condition (if, single line)'
if then echo not reached; fi
__IN__
syntax error: commands are missing between `if' and `then'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty condition (if, multi-line)'
if
then echo not reached; fi
__IN__
syntax error: commands are missing between `if' and `then'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (then, single line)'
if true; then fi
__IN__
syntax error: commands are missing after `then'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (then, multi-line)'
if true; then
fi
__IN__
syntax error: commands are missing after `then'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (else, single line)'
if true; then echo not reached; else fi
__IN__
syntax error: commands are missing after `else'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty body (else, multi-line)'
if true; then echo not reached; else
fi
__IN__
syntax error: commands are missing after `else'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty condition (elif, single line)'
if false; then echo not reached 1; elif then echo not reached 2; fi
__IN__
syntax error: commands are missing between `elif' and `then'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'POSIX: empty condition (elif, multi-line)'
if false; then echo not reached 1; elif
then echo not reached 2; fi
__IN__
syntax error: commands are missing between `elif' and `then'
__ERR__
#'
#`
#'
#`

)

test_Oe -e 2 'unpaired then (direct)'
then
    :; fi
__IN__
syntax error: encountered `then' without a matching `if' or `elif'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired then (after then)'
if echo not reached 1; then echo not reached 2; then
    :; fi
__IN__
syntax error: encountered `then' without a matching `if' or `elif'
syntax error: (maybe you missed `fi'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired fi (direct)'
fi
__IN__
syntax error: encountered `fi' without a matching `if' and/or `then'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired fi (after if)'
if echo not reached; fi
__IN__
syntax error: encountered `fi' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired fi (after elif)'
if echo not reached 1; then echo not reached 2; elif echo not reached 3; fi
__IN__
syntax error: encountered `fi' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired elif (direct)'
elif
    echo not reached 1; then echo not reached 2; fi
__IN__
syntax error: encountered `elif' without a matching `if' and/or `then'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired elif (after if)'
if echo not reached 0; elif
    echo not reached 1; then echo not reached 2; fi
__IN__
syntax error: encountered `elif' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired elif (after then)'
if echo not reached 0; elif
    echo not reached 1; then echo not reached 2; fi
__IN__
syntax error: encountered `elif' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired else (direct)'
else
    echo not reached; fi
__IN__
syntax error: encountered `else' without a matching `if' and/or `then'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired else (after if)'
if echo not reached 0; else
    echo not reached 1; fi
__IN__
syntax error: encountered `else' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'unpaired else (after elif)'
if echo not reached 0; then echo not reached 1; elif echo not reached 2; else
    echo not reached 3; fi
__IN__
syntax error: encountered `else' without a matching `if' and/or `then'
syntax error: (maybe you missed `then'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'missing then-fi (after if)'
if echo not reached
__IN__
syntax error: `then' is missing
syntax error: `fi' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'missing then-fi (after elif)'
if echo not reached 0; then echo not reached 1; elif echo not reached 2
__IN__
syntax error: `then' is missing
syntax error: `fi' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'missing fi (after if-then)'
if echo not reached 0; then echo not reached 1
__IN__
syntax error: `fi' is missing
__ERR__
#'
#`

test_Oe -e 2 'missing fi (after elif-then)'
if echo not reached 0; then echo not reached 1
elif echo not reached 2; then echo not reached 3
__IN__
syntax error: `fi' is missing
__ERR__
#'
#`

test_Oe -e 2 'missing then (after if, in grouping)'
{ if }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `then'?)
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `fi'?)
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

test_Oe -e 2 'missing then (after elif, in grouping)'
{ if :; then :; elif }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `then'?)
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `fi'?)
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

test_Oe -e 2 'missing fi (after then, in grouping)'
{ if :; then }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `fi'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'missing fi (after else, in grouping)'
{ if :; then :; else }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `fi'?)
__ERR__
#'
#`
#'
#`
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
