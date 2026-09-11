import subprocess

from glustercli.metrics import cmdlineparser

GLUSTER_PROCS = [
    "glusterd",
    "glusterfsd",
    "glustershd",
    "glusterfs",
    "python",  # gsyncd, glustereventsd etc
    "ssh",  # gsyncd related ssh connections
]


def get_cmdline(pid):
    args = []
    try:
        with open("/proc/{0}/cmdline".format(pid), "r") as cmdline_file:
            args = cmdline_file.read().strip("\x00").split("\x00")
    except IOError:
        pass

    return args


def local_processes():
    # Run ps command and get all the details for all gluster processes
    # ps --no-header -ww -o pid,pcpu,pmem,rsz,vsz,etimes,comm -C glusterd,..
    # command can be used instead of comm, but if an argument has space then
    # it is a problem
    # for example `mytool "hello world" arg2` will be displayed as
    # `mytool hello world arg2` in ps output
    # Read cmdline from `/proc/<pid>/cmdline` to get full commands
    # Use argparse to parse the output and form the key
    # Example output of ps command:
    # 6959  0.0  0.6 12840 713660  504076 glusterfs
    details = []
    cmd = ["ps",
           "--no-header",  # No header in the output
           "-ww",  # To set unlimited width to avoid crop
           "-o",  # Output Format
           "pid,pcpu,pmem,rsz,vsz,etimes,comm",
           "-C",
           ",".join(GLUSTER_PROCS)]

    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE,
                            universal_newlines=True)
    out, _ = proc.communicate()
    # No records in `ps` output
    if proc.returncode != 0:
        return details

    data = out.strip()

    for line in data.split("\n"):
        # Sample data:
        # 6959  0.0  0.6 12840 713660  504076 glusterfs
        try:
            pid, pcpu, pmem, rsz, vsz, etimes, _ = line.strip().split()
        except ValueError:
            # May be bad ps output, for example
            # 30916  0.0  0.0     0      0       7 python <defunct>
            continue

        args = get_cmdline(int(pid))
        if not args:
            # Unable to collect the cmdline, may be IO error and process died?
            continue

        cmdname = args[0].split("/")[-1]
        func_name = "parse_cmdline_" + cmdname
        details_func = getattr(cmdlineparser, func_name, None)

        if details_func is not None:
            data = details_func(args)
            if data is not None:
                data["percentage_cpu"] = float(pcpu)
                data["percentage_memory"] = float(pmem)
                data["resident_memory"] = int(rsz)
                data["virtual_memory"] = int(vsz)
                data["elapsed_time_sec"] = int(etimes)
                data["pid"] = int(pid)
                details.append(data)

    return details
