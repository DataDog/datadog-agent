# jobs-y.tst: yash-specific test of the jobs & suspend built-ins
../checkfg || skip="true" # %REQUIRETTY%

test_oE 'suspend: suspending' -m
"$TESTEE" -c 'suspend; echo $?'
echo -
fg >/dev/null
__IN__
-
0
__OUT__

test_oE 'jobs: printing jobs' -m +o curstop
"$TESTEE" -c 'suspend; : 1'
"$TESTEE" -c 'suspend; : 2'
"$TESTEE" -c 'suspend; : 3'
jobs
echo \$?=$?
while fg; do :; done >/dev/null 2>&1
__IN__
[1]   Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : 1'
[2] - Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : 2'
[3] + Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : 3'
$?=0
__OUT__

test_oE 'jobs: specifying job IDs' -m +o curstop
"$TESTEE" -c 'suspend; : task-1'
"$TESTEE" -c 'suspend; : task-2'
"$TESTEE" -c 'suspend; : task-3'
jobs %1
echo
jobs 1
echo
jobs %\?task-2
echo
jobs \?task-2
echo \$?=$?
while fg; do :; done >/dev/null 2>&1
__IN__
[1]   Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : task-1'

[1]   Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : task-1'

[2] - Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : task-2'

[2] - Stopped(SIGSTOP)     "${TESTEE}" -c 'suspend; : task-2'
$?=0
__OUT__

test_oE 'exit status of suspended job' -m
"$TESTEE" -cim --norcfile 'echo 1; suspend; echo 2'
kill -l $?
#bg >/dev/null
#wait %
#kill -l $?
fg >/dev/null
__IN__
1
STOP
2
__OUT__

test_Oe -e 1 'non-existing job number'
jobs %100
__IN__
jobs: no such job `%100'
__ERR__
#'
#`

test_Oe -e 1 'non-existing job name'
jobs %no_such_job
__IN__
jobs: no such job `%no_such_job'
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
jobs --no-such=option
__IN__
jobs: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
exec 3>>|4
(exec 3>&- && cat <&4)& # dummy command to be printed by "jobs"
jobs >&-
__IN__

(
posix=true

test_Oe -e 1 'initial % cannot be omitted in POSIX mode'
jobs foo
__IN__
jobs: `foo' is not a valid job specification
__ERR__
#'
#`

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
