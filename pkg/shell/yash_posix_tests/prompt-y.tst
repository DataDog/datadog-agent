# prompt-y.tst: yash-specific test of input processing

user_id="$(id -u)"

mkfifo fifo

(

if [ "$user_id" -eq 0 ]; then
    skip="true"
fi

(
# Detail behavior of prompting and command history is different among shell
# implementations, so we don't test it in input-p.tst.
posix="true"

test_e 'expansion in PS1 (POSIX)' -i +m
PS1='ps1 %'; a=A; echo >&2
PS1='${a} @'; echo >&2
PS1=''; echo >&2
__IN__
$ 
ps1 %
A @
__ERR__

test_e 'PS2' -i +m
PS2='${b} %'; \
echo >&2
b=B; \
echo >&2
true &&
echo >&2; exit
__IN__
$ > 
$  %
$ B %
__ERR__

# It is POSIXly unspecified (1) whether assignment to $PS4 affects the prompt
# for the assignment itself, and (2) how characters are quoted in xtrace. This
# test case tests yash-specific behavior.
test_e 'PS4' -x
echo 0
PS4='ps4 ${x}:' x='${y}'
echo 1; echo '2  2' 3
__IN__
+ echo 0
ps4 ${y}:PS4='ps4 ${x}:' x='${y}'
ps4 ${y}:echo 1
ps4 ${y}:echo '2  2' 3
__ERR__

(
if ! "$TESTEE" -c 'command -bv fc history' >/dev/null; then
    skip="true"
fi

export HISTFILE=/dev/null HISTRMDUP=1

test_e 'job number expansion in PS1' -i +m
PS1='#!$'; echo >&2
echo >&2
echo >&2
echo >&2; exit
__IN__
$ 
#2$
#3$
#3$
__ERR__

test_e 'literal exclamation in PS1' -i +m
PS1='!!$'; echo >&2
echo >&2
echo >&2
echo >&2; exit
__IN__
$ 
!$
!$
!$
__ERR__

)

test_e 'YASH_PSx is ignored in POSIX mode' -i +m
PS1='NO1' YASH_PS1='YES1' PS2='NO2' YASH_PS2='YES2'; echo >&2
\
echo >&2; exit
__IN__
$ 
NO1NO2
__ERR__

test_Oe 'PROMPT_COMMAND is ignored in POSIX mode' -i +m
PROMPT_COMMAND='echo not printed'; echo >&2
echo >&2; exit
__IN__
$ 
$ 
__ERR__

test_Oe 'POST_PROMPT_COMMAND is ignored in POSIX mode' -i +m
POST_PROMPT_COMMAND='echo not printed'; echo >&2
echo >&2; exit
__IN__
$ 
$ 
__ERR__

)

test_e 'YASH_PSx precedes PSx (non-POSIX)' -i +m
PS1='NO1' YASH_PS1='YES1' PS2='NO2' YASH_PS2='YES2'; echo >&2
\
echo >&2; exit
__IN__
$ 
YES1YES2
__ERR__

test_e 'expansion and substitution in PS1' -i +m
PS1='${PWD##"$PWD"}$(echo \?)'; echo >&2
PS1='! !! $ '; echo >&2
echo >&2; exit
__IN__
$ 
?
! !! $ 
__ERR__

# TODO: Test of \[, \], and \f is missing
# \j and \$ are tested in other test cases below
test_e 'backslash notations in PS1' -i +m
PS1='\a \e \n \r $(printf \\\\)\ $'; echo >&2
echo >&2; exit
__IN__
$ 
  
  \ $
__ERR__

# TODO: Test of \j, \[, \], and \f is missing
# \$ is tested in another test case below
test_e 'backslash notations in PS2' -i +m
PS2='\a \e \n \r \\ >'; echo >&2
\
echo >&2; exit
__IN__
$ 
$   
  \ >
__ERR__

test_e '\j in PS1: shows job count' -i +m
PS1='\j$';             echo >&2
{ :& }    2>/dev/null; echo >&2
{ :&&:& } 2>/dev/null; echo >&2
wait $!;               echo >&2
wait;                  echo >&2
                       echo >&2; exit
__IN__
$ 
0$
1$
2$
1$
0$
__ERR__

# This test case occasionally fails, perhaps when the shell did not receive the
# SIGCHLD signal for the 'exec >fifo' child process before the prompt for the
# line containing the wait command. The three sleep commands should mitigate
# this, but if the test still fails, please just retry.
# See: https://osdn.net/tracker.php?id=37560
test_e '\j in PS1 and -b option' -ib +m
PS1='\j$';                   echo >&2
{ exec >fifo& } 2>/dev/null; echo >&2
cat fifo; sleep 0; sleep 0; sleep 0
wait $!;                     echo >&2
                             echo >&2; exit
__IN__
$ 
0$
1$[1] + Done                 exec 1>fifo
0$
0$
__ERR__

test_e 'prompt command' -i +m
PROMPT_COMMAND='printf 1 >&2'; echo >&2
PROMPT_COMMAND=('printf 1 >&2'
'printf 2 >&2; printf 3 >&2; (exit 2)'); echo $? >&2; (exit 1)
echo $? >&2; exit
__IN__
$ 
1$ > 0
123$ 1
__ERR__

test_oe 'value of $COMMAND in post-prompt command' -i +m
POST_PROMPT_COMMAND='printf "[%s]\n" "$COMMAND" >&2'
echo foo\
bar; exit
__IN__
foobar
__OUT__
$ $ [echo foo\]
> [bar; exit]
__ERR__

test_o 'modifying $COMMAND in post-prompt command' -i +m
POST_PROMPT_COMMAND='COMMAND="$COMMAND; echo post"'
echo foo
exit
__IN__
foo
post
__OUT__

test_O 'unsetting $COMMAND in post-prompt command' -i +m
POST_PROMPT_COMMAND='if [ "$COMMAND" != exit ]; then unset COMMAND; fi'
echo foo\
bar
exit
echo not reached
__IN__

test_e '\$ in PS1 and PS2 (non-root)' -i +m
PS1='\$ ' PS2='\$_'; echo >&2
e\
c\
ho >&2; exit
__IN__
$ 
$ $_$_
__ERR__

)

(
if [ "$user_id" -ne 0 ]; then
    skip="true"
fi

test_e '\$ in PS1 and PS2 (root)' -i +m
PS1='\$ ' PS2='\$_'; echo >&2
e\
c\
ho >&2; exit
__IN__
# 
# #_#_
__ERR__

)

(
setup -d

(
if [ "$user_id" -ne 0 ]; then
    skip="true"
fi

test_o 'default prompt strings (root)' -i +m
bracket "$PS1"
bracket "$PS2"
bracket "$PS4"
__IN__
[# ]
[> ]
[+ ]
__OUT__

)

(
if [ "$user_id" -eq 0 ]; then
    skip="true"
fi

test_o 'default prompt strings (non-root)' -i +m
bracket "$PS1"
bracket "$PS2"
bracket "$PS4"
__IN__
[$ ]
[> ]
[+ ]
__OUT__

)

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
