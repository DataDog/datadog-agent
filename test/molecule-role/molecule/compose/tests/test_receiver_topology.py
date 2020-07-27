import json
import os
import re
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('trace-java-demo')


def _component_data(json_data, type_name, external_id_assert_fn, data_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            if data_assert_fn(json.loads(p["TopologyComponent"]["data"])):
                return json.loads(p["TopologyComponent"]["data"])
    return None


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return p["TopologyRelation"]
    return None


def test_java_traces(host):
    def assert_ok():
        c = "curl -H Host:stackstate-books-app -s -o /dev/null -w \"%{http_code}\" http://localhost/stackstate-books-app/listbooks"
        assert host.check_output(c) == "200"

    util.wait_until(assert_ok, 120, 10)

    def assert_topology():
        topo_url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1500"
        data = host.check_output("curl \"%s\"" % topo_url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-traces.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        components = [
            {
                "assertion": "Should find the traefik service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/traefik",
                "data": lambda d: (
                    d["name"] == "traefik" and
                    "serviceType" in d and
                    d["serviceType"] == "traefik"
                )
            },
            {
                "assertion": "Should find the stackstate-books-app traefik service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app",
                "data": lambda d: (
                    d["name"] == "stackstate-books-app" and
                    d["service"] == "traefik" and
                    "serviceType" in d and
                    d["serviceType"] == "traefik"
                )
            },
            {
                "assertion": "Should find the stackstate-books-app traefik service instance",
                "type": "service-instance",
                "external_id": lambda e_id: re.compile(
                    r"urn:service-instance:/stackstate-books-app:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    "stackstate-books-app-" in d["name"] and
                    d["service"] == "traefik" and
                    "serviceType" in d and
                    d["serviceType"] == "traefik"
                )
            },
            {
                "assertion": "Should find the stackstate-authors-app traefik service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-authors-app",
                "data": lambda d: (
                    d["name"] == "stackstate-authors-app" and
                    d["service"] == "traefik" and
                    "serviceType" in d and
                    d["serviceType"] == "traefik"
                )
            },
            {  # TODO: Backend names in Traefik replace . with -, find a way to change the backend name
                "assertion": "Should find the stackstate-authors-app traefik service instance",
                "type": "service-instance",
                "external_id": lambda e_id: re.compile(
                    r"urn:service-instance:/stackstate-authors-app:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    "stackstate-authors-app-" in d["name"] and
                    d["service"] == "traefik" and
                    "serviceType" in d and
                    d["serviceType"] == "traefik"
                )
            },
            {
                "assertion": "Should find the stackstate-authors-app java service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-authors-app",
                "data": lambda d: (
                    d["name"] == "stackstate-authors-app" and
                    d["service"] == "stackstate-authors-app" and
                    "serviceType" in d and
                    d["serviceType"] == "java"
                )
            },
            {
                "assertion": "Should find the stackstate-books-app java service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app",
                "data": lambda d: (
                    d["name"] == "stackstate-books-app" and
                    d["service"] == "stackstate-books-app" and
                    "serviceType" in d and
                    d["serviceType"] == "java"
                )
            },
            {
                "assertion": "Should find the stackstate-authors-app java service instance",
                "type": "service-instance",
                "external_id": lambda e_id: re.compile(
                    r"urn:service-instance:/stackstate-authors-app:/.*:.*:.*").findall(e_id),
                "data": lambda d: (
                    "stackstate-authors-app-" in d["name"] and
                    d["service"] == "stackstate-authors-app" and
                    "serviceType" in d and
                    d["serviceType"] == "java"
                )
            },
            {
                "assertion": "Should find the stackstate-books-app java service",
                "type": "service-instance",
                "external_id": lambda e_id: re.compile(r"urn:service-instance:/stackstate-books-app:/.*:.*:.*").findall(
                    e_id),
                "data": lambda d: (
                    "stackstate-books-app-" in d["name"] and
                    d["service"] == "stackstate-books-app" and
                    "serviceType" in d and
                    d["serviceType"] == "java"
                )
            },
            {
                "assertion": "Should find the postgres service",
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/postgresql:app",
                "data": lambda d: (
                    d["name"] == "postgresql:app" and
                    "serviceType" in d and
                    d["serviceType"] == "postgresql"
                )
            }
        ]

        for c in components:
            print("Running assertion for: " + c["assertion"])
            assert _component_data(
                json_data=json_data,
                type_name=c["type"],
                external_id_assert_fn=c["external_id"],
                data_assert_fn=c["data"],
            ) is not None

        relations = [
            {
                "assertion": "Should find the 'has' relation between the traefik stackstate authors service + service "
                             "instance",
                "type": "has",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-authors-app->urn:service-instance:/stackstate-authors-app:/.*:.*:.*"
                ).findall(e_id),
                "data": lambda d: (
                    d["sourceData"]["service"] == "stackstate-authors-app" and
                    d["targetData"]["service"] == "stackstate-authors-app"
                )
            },
            {
                "assertion": "Should find the 'has' relation between the traefik stackstate books service + service "
                             "instance",
                "type": "has",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-books-app->urn:service-instance:/stackstate-books-app:/.*:.*:.*").findall(
                    e_id),
                "data": lambda d: (
                    d["sourceData"]["service"] == "stackstate-books-app" and
                    d["targetData"]["service"] == "stackstate-books-app"
                )
            },
            {
                "assertion": "Should find the 'has' relation between the java stackstate authors service + service "
                             "instance",
                "type": "has",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-authors-app->urn:service-instance:/stackstate-authors-app:/.*\..*\..*\..*")
                .findall(e_id),
                "data": lambda d: (
                    d["sourceData"]["name"] == "stackstate-authors-app" and
                    "stackstate-authors-app-" in d["targetData"]["name"]
                )
            },
            {
                "assertion": "Should find the 'has' relation between the java stackstate books service + service "
                             "instance",
                "type": "has",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-books-app->urn:service-instance:/stackstate-books-app:/.*\..*\..*\..*"
                ).findall(e_id),
                "data": lambda d: (
                    d["sourceData"]["name"] == "stackstate-books-app" and
                    "stackstate-books-app-" in d["targetData"]["name"]
                )
            },
            # traefik -> books
            {
                "assertion": "Should find the 'calls' relation between traefik and the stackstate books app",
                "type": "trace_call",
                "external_id": lambda e_id: e_id == "urn:service:/traefik->urn:service:/stackstate-books-app",
            },
            {
                "assertion": "Should find the callback 'calls' relation between the stackstate books app and traefik",
                "type": "trace_call",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app->urn:service"
                                                    ":/traefik",
            },
            {
                "assertion": "Should find the 'calls' relation between the stackstate books app and postgresql",
                "type": "trace_call",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app->urn:service:/postgresql:app",
            },
            {
                "assertion": "Should find the 'calls' relation between the stackstate books app instance and postgresql",
                "type": "trace_call",
                "external_id": lambda e_id: re.compile(
                    r"urn:service-instance:/stackstate-books-app:/.*:.*:.*>urn:service:/postgresql:app"
                ).findall(e_id),
            },
            # # traefik -> authors
            {
                "assertion": "Should find the 'calls' relation between traefik and the stackstate authors app",
                "type": "trace_call",
                "external_id": lambda e_id: e_id == "urn:service:/traefik->urn:service:/stackstate-authors-app",
            },
            {
                "assertion": "Should find the 'calls' relation between the stackstate authors app and a stackstate "
                             "authors app instance",
                "type": "trace_call",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-authors-app->urn:service-instance:/stackstate-authors-app:/.*:.*:.*"
                ).findall(e_id),
            },
            {
                "assertion": "Should find the 'calls' relation between the stackstate authors app and postgresql",
                "type": "trace_call",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-authors-app->urn:service:/postgresql:app",
            },
            {
                "assertion": "Should find the 'calls' relation between the stackstate authors app instance and "
                             "postgresql",
                "type": "trace_call",
                "external_id": lambda e_id: re.compile(
                    r"urn:service-instance:/stackstate-authors-app:/.*:.*:.*>urn:service:/postgresql:app"
                ).findall(e_id),
            },
            # # callbacks ?
            {
                "assertion": "Should find the 'calls' relation between the stackstate authors app and traefik",
                "type": "trace_call",
                "external_id": lambda e_id: re.compile(
                    r"urn:service:/stackstate-authors-app->urn:service:/traefik"
                ).findall(e_id),
            },
            # # ?
            {
                "assertion": "Should find the 'calls' relation between the stackstate books app and the stackstate "
                             "authors app",
                "type": "trace_call",
                "external_id": lambda e_id: (
                    e_id == "urn:service:/stackstate-books-app->urn:service:/stackstate-authors-app"
                ),
            },
        ]

        for i, r in enumerate(relations):
            print("Running assertion for: " + r["assertion"])
            assert _relation_data(
                json_data=json_data,
                type_name=r["type"],
                external_id_assert_fn=r["external_id"],
            ) is not None

        #         calls               calls        has                  is module of
        # traefik  -->  traefik:books  -->  books  -->  books-instance     -->        books-process
        #       calls
        # books  -->  postgres
        #                calls
        # books-instance  -->  postgres

        #        ?          calls                 calls          has                    is module of
        # books -> traefik  -->   traefik:authors  -->  authors  -->  authors-instance     -->        authors-process
        #                   calls
        #             books  -->  traefik:authors
        #         calls
        # authors  -->  postgres
        #                  calls
        # authors-instance  -->  postgres

        #                 calls                          calls
        # traefik:authors  -->  traefik -> traefik:books  -->  traefik

    util.wait_until(assert_topology, 30, 3)
