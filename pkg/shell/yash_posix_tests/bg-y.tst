# bg-y.tst: yash-specific test of the bg built-in
../checkfg || skip="true" # %REQUIRETTY%

test_Oe -e 1 'non-job-controlled job (default operand)'
:&
set -m
bg
__IN__
bg: the current job is not a job-controlled job
__ERR__

test_Oe -e 1 'non-job-controlled job (job ID operand)'
:&
set -m
bg %:
__IN__
bg: `%:' is not a job-controlled job
__ERR__
#`

test_Oe -e 1 'no such job (name)' -m
: _no_such_job_&
bg %_no_such_job_
__IN__
bg: no such job `%_no_such_job_'
__ERR__
#`

test_Oe -e 1 'no such job (number)' -m
bg %2
__IN__
bg: no such job `%2'
__ERR__
#`

test_O -d -e 1 'printing to closed stream' -m
:&
bg >&-
__IN__

test_Oe -e 2 'invalid option' -m
bg --no-such-option
__IN__
bg: `--no-such-option' is not a valid option
__ERR__
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
