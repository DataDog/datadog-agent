import cProfile
# import yappi
from pympler import tracker


profile = None
tr = None
summary = None

# def start_cpu_profile():
#     """
#     Start a CPU profile
#     """
#     yappi.start()

# def stop_cpu_profile(filepath):
#     """
#     Stop CPU profile and write it to the filepath
#     """
#     if not yappi.is_running():
#         raise Exception("Profile not started")
#     func_stats = yappi.get_func_stats()
#     # pstats_stats = yappi.convert2pstats(func_stats)
#     # pstats_stats.dump_stats(filepath)
#     func_stats.save(filepath)
#     yappi.stop()
#     yappi.clear_stats()

def start_cpu_profile():
    """
    Start a CPU profile
    """
    global profile
    profile = cProfile.Profile()
    profile.enable()

def first_mem_stats():
    global tr, summary
    tr = tracker.SummaryTracker()
    summary = tr.create_summary()

def print_mem_diff():
    summary.print_diff(summary)

def stop_cpu_profile(filepath):
    """
    Stop CPU profile and write it to the filepath
    """
    global profile
    if profile is None:
        raise Exception("Profile not started")
    profile.disable()
    profile.dump_stats(filepath)
    profile = None


# if __name__ == '__main__':
#     import sys
#     if len(sys.argv) > 1:
#         fstats = yappi.YFuncStats().add(sys.argv[1])
#         fstats.print_all()
#         fstats.save("pstat_profile.prof", type='pstat')
#     else:
#         print "requires at least one argument: path to write profile to"
