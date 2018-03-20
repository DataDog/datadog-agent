# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

from collections import defaultdict
import time
import win32pdh
import _winreg

DATA_TYPE_INT = win32pdh.PDH_FMT_LONG
DATA_TYPE_DOUBLE = win32pdh.PDH_FMT_DOUBLE
DATA_POINT_INTERVAL = 0.10
SINGLE_INSTANCE_KEY = "__single_instance"
class WinPDHCounter(object):
    # store the dictionary of pdh counter names
    pdh_counter_dict = {}

    def __init__(self, class_name, counter_name, log, instance_name = None, machine_name = None, precision=None):
        self.logger = log
        self._get_counter_dictionary()
        class_name_index_list = WinPDHCounter.pdh_counter_dict[class_name]
        if len(class_name_index_list) == 0:
            self.logger.warn("Class %s was not in counter name list, attempting english counter" % class_name)
            self._class_name = class_name
        else:
            if len(class_name_index_list) > 1:
                self.logger.warn("Class %s had multiple (%d) indices, using first" % (class_name, len(class_name_index_list)))
            self._class_name = win32pdh.LookupPerfNameByIndex(None, int(class_name_index_list[0]))

        self._is_single_instance = False
        self.hq = win32pdh.OpenQuery()

        self.counterdict = {}
        if precision is None:
            self._precision = win32pdh.PDH_FMT_DOUBLE
        else:
            self._precision = precision
        counters, instances = win32pdh.EnumObjectItems(None, machine_name, self._class_name, win32pdh.PERF_DETAIL_WIZARD)
        if instance_name is None and len(instances) > 0:
            for inst in instances:
                path = self._make_counter_path(machine_name, counter_name, inst, counters)
                try:
                    self.counterdict[inst] = win32pdh.AddCounter(self.hq, path)
                except:
                    self.logger.fatal("Failed to create counter.  No instances of %s\%s" % (
                        self._class_name, self._counter_name))
                try:
                    self.logger.debug("Path: %s\n" % unicode(path))
                except:
                    # some unicode characters are not translatable here.  Don't fail just
                    # because we couldn't log
                    self.logger.debug("Failed to log path")
                    pass
        else:
            if instance_name is not None:
                # check to see that it's valid
                if len(instances) <= 0:
                    self.logger.error("%s doesn't seem to be a multi-instance counter, but asked for specific instance %s" % (
                        class_name, instance_name
                    ))
                    return
                if instance_name not in instances:
                    self.logger.error("%s is not a counter instance in %s" % (
                        instance_name, class_name
                    ))
                    return
            path = self._make_counter_path(machine_name, counter_name, instance_name, counters)
            try:
                self.logger.debug("Path: %s\n" % unicode(path))
            except:
                # some unicode characters are not translatable here.  Don't fail just
                # because we couldn't log
                self.logger.debug("Failed to log path")
                pass
            try:
                self.counterdict[SINGLE_INSTANCE_KEY] = win32pdh.AddCounter(self.hq, path)
            except:
                self.logger.fatal("Failed to create counter.  No instances of %s\%s" % (
                    self._class_name, self._counter_name))
                raise
            self._is_single_instance = True

    def __del__(self):
        if(self.hq):
            win32pdh.CloseQuery(self.hq)

    def is_single_instance(self):
        return self._is_single_instance

    def get_single_value(self):
        if not self.is_single_instance():
            raise ValueError('counter is not single instance %s %s' % (
                self._class_name, self._counter_name))

        vals = self.get_all_values()
        return vals[SINGLE_INSTANCE_KEY]

    def get_all_values(self):
        ret = {}

        # self will retrieve the list of all object names in the class (i.e. all the network interface
        # names in the class "network interface"
        win32pdh.CollectQueryData(self.hq)

        for inst, counter_handle in self.counterdict.iteritems():
            try:
                t, val = win32pdh.GetFormattedCounterValue(counter_handle, self._precision)
                ret[inst] = val
            except Exception as e:
                # exception usually means self type needs two data points to calculate. Wait
                # a bit and try again
                time.sleep(DATA_POINT_INTERVAL)
                win32pdh.CollectQueryData(self.hq)
                # if we get exception self time, just return it up
                try:
                    t, val = win32pdh.GetFormattedCounterValue(counter_handle, self._precision)
                    ret[inst] = val
                except Exception as e:
                    raise e
        return ret

    def _get_counter_dictionary(self):
        if WinPDHCounter.pdh_counter_dict:
            # already populated
            return

        try:
            val, t = _winreg.QueryValueEx(_winreg.HKEY_PERFORMANCE_DATA, "Counter 009")
        except:
            raise

        # val is an array of strings.  The underlying win32 API returns a list of strings
        # which is the counter name, counter index, counter name, counter index (in windows,
        # a multi-string value)
        #
        # the python implementation translates the multi-string value into an array of strings.
        # the array of strings then becomes
        # array[0] = counter_index_1
        # array[1] = counter_name_1
        # array[2] = counter_index_2
        # array[3] = counter_name_2
        #
        # see https://support.microsoft.com/en-us/help/287159/using-pdh-apis-correctly-in-a-localized-language
        # for more detail

        # create a table of the keys to the counter index, because we want to look up
        # by counter name.
        idx = 0
        idx_max = len(val)
        while idx < idx_max:
            # counter index is idx , counter name is idx + 1
            WinPDHCounter.pdh_counter_dict[val[idx+1]].append(val[idx])
            idx += 2

    def _make_counter_path(self, machine_name, counter_name, instance_name, counters):
        '''
        When handling non english versions, the counters don't work quite as documented.
        This is because strings like "Bytes Sent/sec" might appear multiple times in the
        english master, and might not have mappings for each index.

        Search each index, and make sure the requested counter name actually appears in
        the list of available counters; that's the counter we'll use.
        '''
        counter_name_index_list = WinPDHCounter.pdh_counter_dict[counter_name]
        path = ""
        for index in counter_name_index_list:
            c = win32pdh.LookupPerfNameByIndex(None, int(index))
            if c is None or len(c) == 0:
                self.logger.debug("Index %s not found, skipping" % index)
                continue

            # check to see if this counter is in the list of counters for this class
            if c not in counters:
                try:
                    self.logger.debug("Index %s counter %s not in counter list" % (index, unicode(c)))
                except:
                    # some unicode characters are not translatable here.  Don't fail just
                    # because we couldn't log
                    self.logger.debug("Index %s not in counter list" % index)
                    pass

                continue

            # see if we can create a counter
            try:
                path = win32pdh.MakeCounterPath((machine_name, self._class_name, instance_name, None, 0, c))
                self.logger.debug("Successfully created path %s" % index)
                break
            except:
                try:
                    self.logger.info("Unable to make path with counter %s, trying next available" % unicode(c))
                except:
                    self.logger.info("Unable to make path with counter index %s, trying next available" % index)
                    pass
        return path

