# signal.sh: utility for testing signal actions

# $1 = $LINENO
# $2 = signal name
# $3 = target (shell, child, or exec)
# $4 = context (main, subshell, cmdsub, or async)
# $5 = sender (self or other)
# $6 = interactiveness (+i or -i)
# $7 = job control (+m or -m)
# $8 = action inherited from parent (default or ignored)
# $9 = trap set outside context (keep, clear, ignore or command)
# $10 = trap set inside context (keep, clear, ignore or command)
signal_action_test() (

signal=$2
case $2 in
    (CHLD|CONT|URG|WINCH)
        signal_action=spares
        ;;
    (STOP|TSTP|TTIN|TTOU)
        signal_action=stops
        ;;
    (*)
        signal_action=kills
        case $2 in
            (RTMAX|RTMIN)
                if ! "$TESTEE" -c 'trap : RTMAX RTMIN' 2>/dev/null; then
                    skip="true"
                fi
        esac
        ;;
esac

case $3 in
    (shell)
        target=shell
        enter_target=
        exit_target=
        ;;
    (child)
        target='child process'
        enter_target='"$TESTEE" <<\END'
        exit_target='END'
        ;;
    (exec)
        target='execed process'
        enter_target='exec "$TESTEE" <<\END'
        exit_target='END'
        ;;
esac

context=$4
case $4 in
    (main)
        enter_context=
        exit_context=
        ;;
    (subshell)
        enter_context='('
        exit_context=')'
        ;;
    (cmdsub)
        enter_context='x=$('
        exit_context='); s=$?; ${x:+echo "$x"}; exit $s'
        ;;
    (async)
        enter_context='{'
        exit_context='} & wait $!'
        ;;
esac

sender=$5
case $5 in
    (self)
        send="kill -s $signal \$\$"
        ;;
    (other)
        send="\"\$TESTEE\" -c 'kill -s $signal \$PPID'"
        ;;
esac

interact=$6
monitor=$7

parent_action=$8
case $8 in
    (default)
        initial='initially defaulted'
        ;;
    (ignored)
        initial='initially ignored'
        if ! [ "${skip-}" ]; then
            trap '' $signal
        fi
        ;;
esac

outside_trap=$9
case $9 in
    (keep)
        set_outside_trap=''
        ;;
    (clear)
        set_outside_trap="trap - $signal"
        ;;
    (ignore)
        set_outside_trap="trap '' $signal"
        ;;
    (command)
        set_outside_trap="trap 'echo trapped; trap - $signal' $signal"
        ;;
esac

inside_trap=${10}
case ${10} in
    (keep)
        set_inside_trap=''
        ;;
    (clear)
        set_inside_trap="trap - $signal"
        ;;
    (ignore)
        set_inside_trap="trap '' $signal"
        ;;
    (command)
        set_inside_trap="trap 'echo trapped; trap - $signal' $signal"
        ;;
esac

if [ "$parent_action" = ignored ] && [ "$interact" = +i ]; then
    final_trap=ignore
elif [ "$inside_trap" != keep ]; then
    final_trap=$inside_trap
elif [ "$context" = async ] && [ "$monitor" = +m ] &&
    { [ "$signal" = INT ] || [ "$signal" = QUIT ]; } then
    final_trap=ignore
elif [ "$context" = main ] || [ "$outside_trap" = ignore ]; then
    final_trap=$outside_trap
else
    final_trap=keep
fi

if [ "$target $context $interact" = "shell main -i" ] &&
    case $signal in
        (INT|QUIT|TERM)
            true
            ;;
        (*)
            false
            ;;
    esac; then
    default_action=spares
elif [ "$target $context $monitor" = "shell main -m" ] &&
    case $signal in
        (TSTP|TTIN|TTOU)
            true
            ;;
        (*)
            false
            ;;
    esac; then
    default_action=spares
else
    default_action=$signal_action
fi

case $final_trap in
    (keep)
        if [ "$interact" = -i ] && [ "$outside_trap" != keep ]; then
            reset_action=default
        else
            reset_action=$parent_action
        fi
        case $reset_action in
            (default)
                final_action=$default_action
                ;;
            (ignored)
                final_action=spares
                ;;
        esac
        ;;
    (clear)
        final_action=$default_action
        ;;
    (ignore)
        final_action=spares
        ;;
    (command)
        if [ "$target" = shell ]; then
            final_action='is handled by'
        else
            final_action=$default_action
        fi
        ;;
esac

case $signal in
    (KILL)
        final_action=kills
        ;;
    (STOP)
        final_action=stops
        ;;
esac

case $final_action in
    (spares)
        post='echo ok'
        finish=''
        exec 4<<\END
ok
END
        exit_status=0
        ;;
    ('is handled by')
        post='echo ok'
        finish=''
        exec 4<<\END
trapped
ok
END
        exit_status=0
        ;;
    (stops)
        post='echo continued'
        finish='fg >/dev/null'
        exec 4<<\END
continued
END
        exit_status=0
        ;;
    (kills)
        post='echo not printed'
        finish=''
        exec 4</dev/null
        exit_status=$signal
        ;;
esac

testcase "$1" -e "$exit_status" "SIG$signal $final_action $target "\
"($context, $sender, $interact $monitor, $initial, "\
"$outside_trap -> $inside_trap)" $interact $monitor 3<<__IN__ 5<&-
$set_outside_trap
$enter_context
$set_inside_trap
$enter_target
$send
$post
$exit_target
$exit_context
$finish
__IN__

)

# $1 = LINENO
# $2 = interactiveness (+i or -i)
# $3 = job control (+m or -m)
# $4 = action inherited from parent (default or ignored)
# $5, $6, ... = signals
signal_action_test_combo() {
    n=${1}000 d=$2 e=$3 f=$4
    shift 4

    # Enable job-control to make sure every job-controlling testee starts in
    # the foreground. (See #45760)
    if ! [ "${skip-}" ]; then
        set $e
    fi

    for a in shell child exec; do
    for b in main subshell cmdsub async; do
    for g in keep clear ignore command; do
    for h in keep clear ignore command; do
        s=$1
        if [ $a = shell ]; then
            if [ $g = keep ] && [ $h != keep ]; then
                # This case is the same as $g != keep && $h = keep.
                # Skip the redundant test.
                continue
            fi
        fi
        case $s in (STOP|TTIN|TTOU|TSTP)
            if [ $e = +m ]; then
                # The test implementation only works with -m.
                # For +m, handling of TTIN, TTOU and TSTP is POSIXly unspecified.
                continue
            fi
            # Skip combinations that would freeze the test.
            case $a in
                (shell)
                    continue ;;
                (child)
                    if [ $b != main ]; then continue; fi ;;
                (exec)
                    if [ $b != subshell ]; then continue; fi ;;
            esac
            if [ $a = shell ]; then
                continue
            fi
            if [ $a = child ] && [ $b != main ]; then
                continue
            fi
            if [ $a = exec ] && [ $b != subshell ]; then
                continue
            fi
        esac
        case $s in (KILL|STOP)
            if [ $f = ignored ]; then
                # These signals are not ignorable.
                continue
            fi
        esac
        if [ $s = STOP ] && [ $a != child ]; then
            case $b in (main|cmdsub)
                # This combination would freeze the test.
                continue
            esac
        fi
        if [ $s = CHLD ]; then
            # The OS kernel automatically sends SIGCHLD, which confuses tests.
            if [ $a = child ] || [ $b != main ]; then
                if [ $g = command ]; then
                    continue
                fi
            fi
            if [ $a = child ]; then
                if [ $h = command ]; then
                    continue
                fi
            fi
        fi

        if [ $((n%2)) -eq 0 ] && ! { [ $a = shell ] && [ $b != main ]; }; then
            c=self
        else
            c=other
        fi

        signal_action_test $n $s $a $b $c $d $e $f $g $h

        n=$((n+1))
        shift
        set "$@" "$s"
    done
    done
    done
    done

    set +m
}

# vim: set ft=sh ts=8 sts=4 sw=4 et:
