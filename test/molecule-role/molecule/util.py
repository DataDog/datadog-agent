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


def assert_topology_events(host, test_name, topic, expected_topology_events):
    url = "http://localhost:7070/api/topic/%s?limit=1000" % topic

    def wait_for_topology_events():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-%s-%s.json" % (test_name, topic), 'w') as f:
            json.dump(json_data, f, indent=4)

        def _topology_event_data(event):
            for message in json_data["messages"]:
                p = message["message"]
                if "TopologyEvent" in p:
                    _data = p["TopologyEvent"]
                    if _data == dict(_data, **event):
                        return _data
            return None

        for t_e in expected_topology_events:
            print("Running assertion for: " + t_e["assertion"])
            assert _topology_event_data(t_e["event"]) is not None

    wait_until(wait_for_topology_events, 60, 3)


def assert_topology(host, test_name, topic, expected_components):
    def assert_topology():
        topo_url = "http://localhost:7070/api/topic/%s?limit=1500" % topic
        data = host.check_output('curl "{}"'.format(topo_url))
        json_data = json.loads(data)
        with open("./topic-%s-%s.json" % (test_name, topic), 'w') as f:
            json.dump(json_data, f, indent=4)

        for c in expected_components:
            print("Running assertion for: " + c["assertion"])
            assert component_data(
                json_data=json_data,
                type_name=c["type"],
                external_id_assert_fn=c["external_id"],
                data_assert_fn=c["data"],
            ) is not None

    wait_until(assert_topology, 30, 3)


def assert_metrics(host, test_name, expected_metrics):
    hostname = host.ansible.get_variables()["inventory_hostname"]
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-%s-sts-multi-metrics.json" % test_name, 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys(m_host):
            return set(
                ''.join(message["message"]["MultiMetric"]["values"].keys())
                for message in json_data["messages"]
                if message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                message["message"]["MultiMetric"]["host"] == m_host
            )

        assert all([expected_metric for expected_metric in expected_metrics if expected_metric in get_keys(hostname)])

    wait_until(wait_for_metrics, 180, 3)


def component_data(json_data, type_name, external_id_assert_fn, data_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
            external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            data = json.loads(p["TopologyComponent"]["data"])
            if data and data_assert_fn(data):
                return p["TopologyComponent"]["externalId"]
    return None


def relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return json.loads(p["TopologyRelation"]["data"])
    return None


def event_data(event, json_data, hostname):
    for message in json_data["messages"]:
        p = message["message"]
        if "GenericEvent" in p and p["GenericEvent"]["host"] == hostname:
            _data = p["GenericEvent"]
            if _data == dict(_data, **event):
                return _data
    return None
