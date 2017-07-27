"""
Implementation of WMI calculators a few `CounterType`(s).


Protocol.

    Use MSDN to lookup the class, property and counter type to determine the
    appropriate calculator.

    For example, "Win32_PerfRawData_PerfOS_Memory" is located at:
      https://msdn.microsoft.com/en-us/library/aa394314(v=vs.85).aspx
    and the "CacheBytes" property has a CounterType = 65792 which
    can be determined from:
      https://msdn.microsoft.com/en-us/library/aa389383(v=vs.85).aspx
    The CounterType 65792 (PERF_COUNTER_LARGE_RAWCOUNT) is defined here:
      https://technet.microsoft.com/en-us/library/cc780300(v=ws.10).aspx

    From: https://msdn.microsoft.com/en-us/library/aa389383(v=vs.85).aspx

Original discussion thread: https://github.com/DataDog/dd-agent/issues/1952
Credits to @TheCloudlessSky (https://github.com/TheCloudlessSky)
"""

_counter_type_calculators = {}


class UndefinedCalculator(Exception):
    """
    No calculator is defined for the given CounterType.
    """
    pass


def calculator(counter_type):
    """
    A decorator that assign a counter_type to its calculator.
    """
    def set_calculator(func):
        _counter_type_calculators[counter_type] = func
        return func
    return set_calculator


def get_calculator(counter_type):
    """
    Return the calculator associated with the counter_type when it exists.

    Raise a UndefinedCalculator exception otherwise.
    """
    try:
        return _counter_type_calculators[counter_type]
    except KeyError:
        raise UndefinedCalculator


def get_raw(previous, current, property_name):
    """
    Returns the vanilla RAW property value.

    Not associated with any counter_type. Used to fallback when no calculator
    is defined for a given counter_type.
    """
    return current[property_name]


@calculator(65536)
def calculate_perf_counter_rawcount(previous, current, property_name):
    """
    PERF_COUNTER_RAWCOUNT

    https://technet.microsoft.com/en-us/library/cc757032(v=ws.10).aspx
    """
    return current[property_name]


@calculator(65792)
def calculate_perf_counter_large_rawcount(previous, current, property_name):
    """
    PERF_COUNTER_LARGE_RAWCOUNT

    https://technet.microsoft.com/en-us/library/cc780300(v=ws.10).aspx
    """
    return current[property_name]


@calculator(542180608)
def calculate_perf_100nsec_timer(previous, current, property_name):
    """
    PERF_100NSEC_TIMER

    https://technet.microsoft.com/en-us/library/cc728274(v=ws.10).aspx
    """
    n0 = previous[property_name]
    n1 = current[property_name]
    d0 = previous["Timestamp_Sys100NS"]
    d1 = current["Timestamp_Sys100NS"]

    if n0 is None or n1 is None:
        return

    return (n1 - n0) / (d1 - d0) * 100


@calculator(272696576)
def calculate_perf_counter_bulk_count(previous, current, property_name):
    """
    PERF_COUNTER_BULK_COUNT

    https://technet.microsoft.com/en-us/library/cc757486(v=ws.10).aspx
    """
    n0 = previous[property_name]
    n1 = current[property_name]
    d0 = previous["Timestamp_Sys100NS"]
    d1 = current["Timestamp_Sys100NS"]
    f = current["Frequency_Sys100NS"]

    if n0 is None or n1 is None:
        return

    return (n1 - n0) / ((d1 - d0) / f)


@calculator(272696320)
def calculate_perf_counter_counter(previous, current, property_name):
    """
    PERF_COUNTER_COUNTER

    https://technet.microsoft.com/en-us/library/cc740048(v=ws.10).aspx
    """
    n0 = previous[property_name]
    n1 = current[property_name]
    d0 = previous["Timestamp_Sys100NS"]
    d1 = current["Timestamp_Sys100NS"]
    f = current["Frequency_Sys100NS"]

    if n0 is None or n1 is None:
        return

    return (n1 - n0) / ((d1 - d0) / f)
