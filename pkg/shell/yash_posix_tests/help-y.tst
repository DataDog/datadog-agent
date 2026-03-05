# help-y.tst: yash-specific test of the help built-in

if ! testee --version --verbose | grep -Fqx ' * help'; then
    skip="true"
fi

test_oE -e 0 'help is an elective built-in'
command -V help
__IN__
help: an elective built-in
__OUT__

test_oE -e 0 'without arguments, the help for the help itself is printed'
help
__IN__
help: print usage of built-in commands

Syntax:
	help [built-in...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of alias'
help alias
__IN__
alias: define or print aliases

Syntax:
	alias [-gp] [name[=value]...]

Options:
	-g       --global
	-p       --prefix
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv array' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of array'
help array
__IN__
array: manipulate an array

Syntax:
	array                  # print arrays
	array name [value...]  # set array values
	array -d name [index...]
	array -i name index [value...]
	array -s name index value

Options:
	-d       --delete
	-i       --insert
	-s       --set
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of bg'
help bg
__IN__
bg: run jobs in the background

Syntax:
	bg [job...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv bindkey' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of bindkey'
help bindkey
__IN__
bindkey: set or print key bindings for line-editing

Syntax:
	bindkey -aev [key_sequence [command]]
	bindkey -l

Options:
	-v       --vi-insert
	-a       --vi-command
	-e       --emacs
	-l       --list
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of break'
help break
__IN__
break: exit a loop

Syntax:
	break [count]
	break -i

Options:
	-i       --iteration
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of cd'
help cd
__IN__
cd: change the working directory

Syntax:
	cd [-L|-P [-e]] [directory]

Options:
	-d ...   --default-directory=...
	-e       --ensure-pwd
	-L       --logical
	-P       --physical
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of colon'
help :
__IN__
:: do nothing

Syntax:
	: [...]

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of command'
help command
__IN__
command: execute or identify a command

Syntax:
	command [-befp] command [argument...]
	command -v|-V [-abefkp] command...

Options:
	-a       --alias
	-b       --builtin-command
	-e       --external-command
	-f       --function
	-k       --keyword
	-p       --standard-path
	-v       --identify
	-V       --verbose-identify
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv complete' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of complete'
help complete
__IN__
complete: generate completion candidates

Syntax:
	complete [-A pattern] [-R pattern] [-T] [-P prefix] [-S suffix] \
	         [-abcdfghjkuv] [[-O] [-D description] words...]

Options:
	-A ...   --accept=...
	-a       --alias
	         --array-variable
	         --bindkey
	-b       --builtin-command
	-c       --command
	-D ...   --description=...
	-d       --directory
	         --dirstack-index
	         --elective-builtin
	         --executable-file
	         --extension-builtin
	         --external-command
	-f       --file
	         --finished-job
	         --function
	         --global-alias
	-g       --group
	         --help
	-h       --hostname
	-j       --job
	-k       --keyword
	         --mandatory-builtin
	-T       --no-termination
	         --normal-alias
	-O       --option
	-P ...   --prefix=...
	         --regular-builtin
	-R ...   --reject=...
	         --running-job
	         --scalar-variable
	         --semi-special-builtin
	         --signal
	         --special-builtin
	         --stopped-job
	         --substitutive-builtin
	-S ...   --suffix=...
	-u       --username
	-v       --variable

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of continue'
help continue
__IN__
continue: continue a loop

Syntax:
	continue [count]
	continue -i

Options:
	-i       --iteration
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv dirs' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of dirs'
help dirs
__IN__
dirs: print the directory stack

Syntax:
	dirs [-cv] [index...]

Options:
	-c       --clear
	-v       --verbose
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of disown'
help disown
__IN__
disown: disown jobs

Syntax:
	disown [job...]
	disown -a

Options:
	-a       --all
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of dot'
help .
__IN__
.: read a file and execute commands

Syntax:
	. [-AL] file [argument...]

Options:
	-A       --no-alias
	-L       --autoload
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv echo' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of echo'
help echo
__IN__
echo: print arguments

Syntax:
	echo [string...]

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of eval'
help eval
__IN__
eval: evaluate arguments as a command

Syntax:
	eval [-i] [argument...]

Options:
	-i       --iteration
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of exec'
help exec
__IN__
exec: replace the shell process with an external command

Syntax:
	exec [-cf] [-a name] [command [argument...]]

Options:
	-a ...   --as=...
	-c       --clear
	-f       --force
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of exit'
help exit
__IN__
exit: exit the shell

Syntax:
	exit [-f] [exit_status]

Options:
	-f       --force
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of export'
help export
__IN__
export: export variables as environment variables

Syntax:
	export [-prX] [name[=value]...]

Options:
	-f       --functions
	-g       --global
	-p       --print
	-r       --readonly
	-x       --export
	-X       --unexport
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of false'
help false
__IN__
false: do nothing unsuccessfully

Syntax:
	false

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv fc' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of fc'
help fc
__IN__
fc: list or re-execute command history

Syntax:
	fc [-qr] [-e editor] [first [last]]
	fc -s [-q] [old=new] [first]
	fc -l [-nrv] [first [last]]

Options:
	-e ...   --editor=...
	-l       --list
	-n       --no-numbers
	-q       --quiet
	-r       --reverse
	-s       --silent
	-v       --verbose
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of fg'
help fg
__IN__
fg: run jobs in the foreground

Syntax:
	fg [job...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of getopts'
help getopts
__IN__
getopts: parse command options

Syntax:
	getopts options variable [argument...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of hash'
help hash
__IN__
hash: remember, forget, or report command locations

Syntax:
	hash command...
	hash -r [command...]
	hash [-a]  # print remembered paths
	hash -d user...
	hash -d -r [user...]
	hash -d  # print remembered paths

Options:
	-a       --all
	-d       --directory
	-r       --remove
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of help'
help help
__IN__
help: print usage of built-in commands

Syntax:
	help [built-in...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv history' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of history'
help history
__IN__
history: manage command history

Syntax:
	history [-cF] [-d entry] [-s command] [-r file] [-w file] [count]

Options:
	-c       --clear
	-d ...   --delete=...
	-r ...   --read=...
	-s ...   --set=...
	-w ...   --write=...
	-F       --flush-file
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of jobs'
help jobs
__IN__
jobs: print info about jobs

Syntax:
	jobs [-lnprs] [job...]

Options:
	-l       --verbose
	-n       --new
	-p       --pgid-only
	-r       --running-only
	-s       --stopped-only
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of kill'
help kill
__IN__
kill: send a signal to processes

Syntax:
	kill [-signal|-s signal|-n number] process...
	kill -l [-v] [number...]

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of local'
help local
__IN__
local: set or print local variables

Syntax:
	local [-prxX] [name[=value]...]

Options:
	-p       --print
	-r       --readonly
	-x       --export
	-X       --unexport
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv popd' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of popd'
help popd
__IN__
popd: pop a directory from the directory stack

Syntax:
	popd [index]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

)

(
if ! testee -c 'command -bv printf' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of printf'
help printf
__IN__
printf: print a formatted string

Syntax:
	printf format [value...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

)

(
if ! testee -c 'command -bv pushd' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of pushd'
help pushd
__IN__
pushd: push a directory into the directory stack

Syntax:
	pushd [-L|-P [-e]] [directory]

Options:
	-D       --remove-duplicates
	-d ...   --default-directory=...
	-e       --ensure-pwd
	-L       --logical
	-P       --physical
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of pwd'
help pwd
__IN__
pwd: print the working directory

Syntax:
	pwd [-L|-P]

Options:
	-L       --logical
	-P       --physical
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of read'
help read
__IN__
read: read a line from the standard input

Syntax:
	read [-Aer] [-d delimiter] [-P|-p prompt] variable...

Options:
	-A       --array
	-d ...   --delimiter=...
	-e       --line-editing
	-P       --ps1
	-p ...   --prompt=...
	-r       --raw-mode
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of readonly'
help readonly
__IN__
readonly: make variables read-only

Syntax:
	readonly [-fpxX] [name[=value]...]

Options:
	-f       --functions
	-g       --global
	-p       --print
	-r       --readonly
	-x       --export
	-X       --unexport
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of return'
help return
__IN__
return: return from a function or script

Syntax:
	return [-n] [exit_status]

Options:
	-n       --no-return
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee --version --verbose | grep -Fqx ' * lineedit'; then
    skip="true"
fi

test_oE -e 0 'help of set'
help set
__IN__
set: set shell options and positional parameters

Syntax:
	set [option...] [--] [new_positional_parameter...]
	set -o|+o  # print current settings

Options:
	-a       -o allexport
	         -o braceexpand
	         -o caseglob
	+C       -o clobber
	-c       -o cmdline
	         -o curasync
	         -o curbg
	         -o curstop
	         -o dotglob
	         -o emacs
	         -o emptylastfield
	-e       -o errexit
	         -o errreturn
	+n       -o exec
	         -o extendedglob
	         -o forlocal
	+f       -o glob
	-h       -o hashondef
	         -o histspace
	         -o ignoreeof
	-i       -o interactive
	         -o lealwaysrp
	         -o lecompdebug
	         -o leconvmeta
	         -o lenoconvmeta
	         -o lepredict
	         -o lepredictempty
	         -o lepromptsp
	         -o letrimright
	         -o levisiblebell
	         -o log
	-l       -o login
	         -o markdirs
	-m       -o monitor
	-b       -o notify
	         -o notifyle
	         -o nullglob
	         -o pipefail
	         -o posixlycorrect
	-s       -o stdin
	         -o traceall
	+u       -o unset
	-v       -o verbose
	         -o vi
	-x       -o xtrace

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of shift'
help shift
__IN__
shift: remove some positional parameters or array elements

Syntax:
	shift [-A array_name] [count]

Options:
	-A ...   --array=...
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of suspend'
help suspend
__IN__
suspend: suspend the shell

Syntax:
	suspend [-f]

Options:
	-f       --force
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv test' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'help of test'
help test
__IN__
test: evaluate a conditional expression

Syntax:
	test expression
	[ expression ]

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of ['
help [
__IN__
[: evaluate a conditional expression

Syntax:
	test expression
	[ expression ]

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of times'
help times
__IN__
times: print CPU time usage

Syntax:
	times

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of trap'
help trap
__IN__
trap: set or print signal handlers

Syntax:
	trap [action signal...]
	trap signal_number [signal...]
	trap -p [signal...]

Options:
	-p       --print
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of true'
help true
__IN__
true: do nothing successfully

Syntax:
	true

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of type'
help type
__IN__
type: identify a command

Syntax:
	type command...

Options:
	-a       --alias
	-b       --builtin-command
	-e       --external-command
	-f       --function
	-k       --keyword
	-p       --standard-path
	-v       --identify
	-V       --verbose-identify
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of typeset'
help typeset
__IN__
typeset: set or print variables

Syntax:
	typeset [-fgprxX] [name[=value]...]

Options:
	-f       --functions
	-g       --global
	-p       --print
	-r       --readonly
	-x       --export
	-X       --unexport
	         --help

Try `man yash' for details.
__OUT__
#`

(
if ! testee -c 'command -bv ulimit' >/dev/null; then
    skip="true"
fi

test_x -e 0 'help of ulimit: exit status'
help ulimit
__IN__

test_oE 'help of ulimit: output'
help ulimit | grep -v '^	-[eilmqruvx]'
__IN__
ulimit: set or print a resource limitation

Syntax:
	ulimit -a [-H|-S]
	ulimit [-H|-S] [-efilnqrstuvx] [limit]

Options:
	-H       --hard
	-S       --soft
	-a       --all
	-c       --core
	-d       --data
	-f       --fsize
	-n       --nofile
	-s       --stack
	-t       --cpu
	         --help

Try `man yash' for details.
__OUT__
#`

)

test_oE -e 0 'help of umask'
help umask
__IN__
umask: print or set the file creation mask

Syntax:
	umask mode
	umask [-S]

Options:
	-S       --symbolic
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of unalias'
help unalias
__IN__
unalias: undefine aliases

Syntax:
	unalias name...
	unalias -a

Options:
	-a       --all
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of unset'
help unset
__IN__
unset: remove variables or functions

Syntax:
	unset [-fv] [name...]

Options:
	-f       --functions
	-v       --variables
	         --help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'help of wait'
help wait
__IN__
wait: wait for jobs to terminate

Syntax:
	wait [job or process_id...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_oE -e 0 'specifying many operands'
help true false help
__IN__
true: do nothing successfully

Syntax:
	true

false: do nothing unsuccessfully

Syntax:
	false

help: print usage of built-in commands

Syntax:
	help [built-in...]

Options:
	--help

Try `man yash' for details.
__OUT__
#`

test_Oe -e n 'invalid option'
help --no-such-option
__IN__
help: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e n 'invalid operand'
help XXX
__IN__
help: no such built-in `XXX'
__ERR__
#`

test_O -d -e 127 'help built-in is unavailable in POSIX mode' --posix
echo echo not reached > help
chmod a+x help
PATH=$PWD:$PATH
help --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
