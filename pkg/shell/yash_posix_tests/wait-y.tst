# wait-y.tst: yash-specific test of the wait built-in
../checkfg || skip="true" # %REQUIRETTY%

mkfifo sync

test_x -e 1 'invalid job specification overrides exit status of valid job (1)'
exit 42 &
wait X $!
__IN__

test_x -e 1 'invalid job specification overrides exit status of valid job (2)'
exit 42 &
wait $! X
__IN__

test_o -e 0 'awaited job is printed (with operand, -im, non-POSIX)' -im
# The "jobs" command ensures the "wait" command does not print "Running".
>sync& jobs; cat sync; echo -; wait
__IN__
[1] + Running              1>sync
-
[1] + Done                 1>sync
__OUT__

test_O -e 0 'awaited job is not printed (with operand, -i +m)' -i +m
>sync& cat sync; cat /dev/null; wait %
__IN__

test_O -e 0 'awaited job is not printed (with operand, +i -m)' -m
>sync& cat sync; cat /dev/null; wait %
__IN__

test_O -e 0 'awaited job is not printed (with operand, -im, POSIX)' -im --posix
>sync& cat sync; cat /dev/null; wait %
__IN__

test_o -e 0 'awaited job is printed (w/o operand, -im, non-POSIX)' -im
# The "jobs" command ensures the "wait" command does not print "Running".
>sync& jobs; cat sync; echo -; wait
__IN__
[1] + Running              1>sync
-
[1] + Done                 1>sync
__OUT__

test_O -e 0 'awaited job is not printed (w/o operand, -i +m)' -i +m
>sync& cat sync; cat /dev/null; wait
__IN__

test_O -e 0 'awaited job is not printed (w/o operand, +i -m)' -m
>sync& cat sync; cat /dev/null; wait
__IN__

test_O -e 0 'awaited job is not printed (w/o operand, -im, POSIX)' -im --posix
>sync& cat sync; cat /dev/null; wait
__IN__

test_x -e 127 'job is forgotten after awaited' -im
exec >sync && exit 17 &
pid=$!
cat sync
:
:
:
wait
wait $pid
__IN__

test_Oe -e 2 'invalid option --xxx'
wait --no-such=option
__IN__
wait: `--no-such=option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
