import json
import os
import re
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('trace-java-demo')


def _component_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            return json.loads(p["TopologyComponent"]["data"])
    return None


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return json.loads(p["TopologyRelation"]["data"])
    return None


def test_java_traces(host):
    def assert_ok():
        c = "curl -H Host:stackstate-books-app.docker.localhost -s -o /dev/null -w \"%{http_code}\" http://localhost/stackstate-books-app/listbooks"
        assert host.check_output(c) == "200"

    util.wait_until(assert_ok, 120, 10)

    def assert_topology():
        topo_url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=5000"
        data = host.check_output("curl \"%s\"" % topo_url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-traces.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        components = [
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/traefik",
                "data": lambda d: d["name"] == "traefik"
            },
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/traefik:stackstate-authors-app.docker.localhost",
                "data": lambda d: d["name"] == "stackstate-authors-app" and "urn:service:/stackstate-authors-app" in d["identifiers"] and d["service"] == "stackstate-authors-app"
            },
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/traefik:stackstate-books-app.docker.localhost",
                "data": lambda d: d["name"] == "stackstate-books-app" and "urn:service:/stackstate-books-app" in d["identifiers"] and d["service"] == "stackstate-books-app"
            },
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-authors-app",
                "data": lambda d: d["name"] == "stackstate-authors-app"
            },
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app",
                "data": lambda d: d["name"] == "stackstate-books-app"
            },
            {
                "type": "service",
                "external_id": lambda e_id: e_id == "urn:service:/postgresql:app",
                "data": lambda d: d["name"] == "postgresql:app" and d["serviceType"] == "postgresql"
            },
            {
                "type": "service-instance",
                "external_id": lambda e_id: re.compile("urn:service-instance:/stackstate-authors-app:/.*:.*:.*").findall(e_id),
                "data": lambda d: d["service"] == "stackstate-authors-app"
            },
            {
                "type": "service-instance",
                "external_id": lambda e_id: re.compile("urn:service-instance:/stackstate-books-app:/.*:.*:.*").findall(e_id),
                "data": lambda d: d["service"] == "stackstate-books-app"
            },
        ]

        for c in components:
            assert c["data"](
                _component_data(
                    json_data=json_data,
                    type_name=c["type"],
                    external_id_assert_fn=c["external_id"],
                )
            )

        relations = [
            {
                "type": "has",
                "external_id": lambda e_id: re.compile("urn:service:/stackstate-authors-app->urn:service-instance:/stackstate-authors-app:/.*:.*:.*").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-authors-app" and d["targetData"]["service"] == "stackstate-authors-app"
            },
            {
                "type": "has",
                "external_id": lambda e_id: re.compile("urn:service:/stackstate-books-app->urn:service-instance:/stackstate-books-app:/.*:.*:.*").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-books-app" and d["targetData"]["service"] == "stackstate-books-app"
            },
            {
                "type": "is_module_of",
                "external_id": lambda e_id: re.compile(r"urn:service-instance:/stackstate-authors-app:/(.*):(.*):(.*)->urn:process:/\1:\2:\3").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-authors-app" and d["targetData"]["pid"] is not None
            },
            {
                "type": "is_module_of",
                "external_id": lambda e_id: re.compile(r"urn:service-instance:/stackstate-books-app:/(.*):(.*):(.*)->urn:process:/\1:\2:\3").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-books-app" and d["targetData"]["pid"] is not None
            },
            # traefik -> books
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/traefik->urn:service:/traefik:stackstate-books-app.docker.localhost",
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "traefik",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/traefik:stackstate-books-app.docker.localhost->urn:service:/stackstate-books-app",
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "stackstate-books-app",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app->urn:service:/postgresql:app",
                "data": lambda d: d["sourceData"]["service"] == "stackstate-books-app" and d["targetData"]["service"] == "postgresql",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: re.compile("urn:service-instance:/stackstate-books-app:/.*:.*:.*->urn:service:/postgresql:app").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-books-app" and d["targetData"]["service"] == "postgresql",
            },
            # traefik -> authors
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/traefik->urn:service:/traefik:stackstate-authors-app.docker.localhost",
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "traefik",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/traefik:stackstate-authors-app.docker.localhost->urn:service:/stackstate-authors-app",
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "stackstate-authors-app",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-authors-app->urn:service:/postgresql:app",
                "data": lambda d: d["sourceData"]["service"] == "stackstate-authors-app" and d["targetData"]["service"] == "postgresql",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: re.compile("urn:service-instance:/stackstate-authors-app:/.*:.*:.*->urn:service:/postgresql:app").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "stackstate-authors-app" and d["targetData"]["service"] == "postgresql",
            },
            # callbacks ?
            {
                "type": "calls",
                "external_id": lambda e_id: re.compile("urn:service:/traefik:stackstate-authors-app.docker.localhost->urn:service:/traefik").findall(e_id),
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "traefik",
            },
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/traefik:stackstate-books-app.docker.localhost->urn:service:/traefik",
                "data": lambda d: d["sourceData"]["service"] == "traefik" and d["targetData"]["service"] == "traefik",
            },
            # ?
            {
                "type": "calls",
                "external_id": lambda e_id: e_id == "urn:service:/stackstate-books-app->urn:service:/traefik:stackstate-authors-app.docker.localhost",
                "data": lambda d: d["sourceData"]["service"] == "stackstate-books-app" and d["targetData"]["service"] == "traefik",
            },
        ]

        for i, r in enumerate(relations):
            print(i)
            assert r["data"](
                _relation_data(
                    json_data=json_data,
                    type_name=r["type"],
                    external_id_assert_fn=r["external_id"],
                )
            )

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
