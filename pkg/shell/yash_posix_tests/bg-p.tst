# bg-p.tst: test of the bg built-in for any POSIX-compliant shell
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

cat >job1 <<\__END__
exec sh -c 'kill -s STOP $$; echo'
__END__

chmod a+x job1
ln job1 job2

test_O -d -e n 'bg cannot be used when job control is disabled'
set -m
:&
set +m
bg
__IN__

test_o 'default operand chooses most recently suspended job' -m
:&
sh -c 'kill -s STOP $$; echo 1'
bg >/dev/null
wait
__IN__
1
__OUT__

test_OE 'already running job is ignored' -m
while kill -s CONT $$; do sleep 1; done &
bg >/dev/null
kill %
__IN__

test_O -e 17 'resumed job is awaitable' -m
sh -c 'kill -s STOP $$; exit 17'
bg >/dev/null
wait %
__IN__

test_o 'resumed job is in background' -m
sh -c 'kill -s STOP $$; ../checkfg || echo bg'
bg >/dev/null
wait %
__IN__
bg
__OUT__

test_o 'specifying job ID' -m
./job1
./job2
echo -
bg %./job1 >/dev/null
bg %./job2 >/dev/null
wait
__IN__
-


__OUT__

test_o 'specifying more than one job ID' -m
./job1
./job2
echo -
bg %./job1 %./job2 >/dev/null
wait
__IN__
-


__OUT__

test_O -e 0 'bg prints resumed job' -m
sleep 1&
bg >bg.out
grep -q '^\[[[:digit:]][[:digit:]]*][[:blank:]]*sleep 1' bg.out
__IN__

test_O -e 17 'bg updates $!' -m
sh -c 'kill -s STOP $$; exit 17'
bg >/dev/null
wait $!
__IN__

test_O -e 0 'exit status of bg' -m
sh -c 'kill -s STOP $$; exit 17'
bg >/dev/null
__IN__

test_O -d -e n 'no existing job' -m
bg
__IN__

test_O -d -e n 'no such job' -m
sh -c 'kill -s STOP $$'
bg %_no_such_job_
exit_status=$?
fg >/dev/null
exit $exit_status
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
