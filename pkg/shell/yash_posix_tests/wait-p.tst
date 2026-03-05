# wait-p.tst: test of the wait built-in for any POSIX-compliant shell
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

test_oE 'waiting for all jobs (+m)'
echo a > a& echo b > b& echo c > c& exit 1&
wait
cat a b c
__IN__
a
b
c
__OUT__

test_OE -e 11 'waiting for specific single job (+m)'
exit 11&
wait $!
__IN__

test_OE -e 1 'waiting for specific many jobs (+m)'
exit 1& p1=$!
exit 2& p2=$!
exit 3& p3=$!
wait $p3 $p2 $p1
__IN__

test_OE -e 127 'waiting for unknown job (+m)'
exit 1&
wait $! $(($!+1))
__IN__

test_OE -e 127 'jobs are not inherited to subshells (+m, -s)'
exit 1&
p=$!
(wait $p)
__IN__

test_OE -e 127 'jobs are not inherited to subshells (+m, -c)' \
    -c 'exit 1& p=$!; (wait $p)'
__IN__

test_OE -e 1 'jobs are not propagated from subshells (+m)'
exit 1&
(exit 2&)
wait $!
__IN__

test_oE 'waiting for all jobs (-m)' -m
echo a > a& echo b > b& echo c > c& exit 1&
wait
cat a b c
__IN__
a
b
c
__OUT__

test_OE -e 11 'waiting for specific single job (-m)' -m
exit 11&
wait $!
__IN__

test_OE -e 1 'waiting for specific many jobs (-m)' -m
exit 1& p1=$!
exit 2& p2=$!
exit 3& p3=$!
wait $p3 $p2 $p1
__IN__

test_OE -e 127 'waiting for unknown job (-m)' -m
exit 1&
wait $! $(($!+1))
__IN__

test_oE -e 11 'specifying job ID' -m
cat /dev/null&
echo 1&
exit 11&
wait %echo %exit
__IN__
1
__OUT__

test_OE -e 127 'jobs are not inherited to subshells (-m, -s)' -m
exit 1&
p=$!
(wait $p)
__IN__

test_OE -e 127 'jobs are not inherited to subshells (+m, -c)' \
    -cm 'exit 1& p=$!; (wait $p)'
__IN__

test_OE -e 1 'jobs are not propagated from subshells (-m)' -m
exit 1&
(exit 2&)
wait $!
__IN__

test_oE 'trap interrupts wait' -m
interrupted=false
trap 'interrupted=true' USR1
while kill -s 0 $$; do kill -s USR1 $$; done&
# The asynchronous job should eventually interrupt the wait.
wait
status=$?
echo interrupted=$interrupted $((status > 128))
kill -l $status
trap '' USR1
# Now the job should be still running. Kill it.
kill -s USR2 %
wait
echo waited $?
__IN__
interrupted=true 1
USR1
waited 0
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
