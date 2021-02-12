import time
import json


def wait_until(someaction, timeout, period=0.25, *args, **kwargs):
    mustend = time.time() + timeout
    while True:
        try:
            someaction(*args, **kwargs)
            return
        except:
            if time.time() >= mustend:
                print("Waiting timed out after %d" % timeout)
                raise
            time.sleep(period)


def component_data(json_data, type_name, external_id_assert_fn, data_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
            external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            data = json.loads(p["TopologyComponent"]["data"])
            if data and data_assert_fn(data):
                return data
    return None


def event_data(event, json_data, hostname):
    for message in json_data["messages"]:
        p = message["message"]
        if "GenericEvent" in p and p["GenericEvent"]["host"] == hostname:
            _data = p["GenericEvent"]
            if _data == dict(_data, **event):
                return _data
    return None
