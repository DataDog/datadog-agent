import json
import logging
import os
import sys
import zlib
from os import path

import monitoring
import pymongo
from flask import Flask, Response, jsonify, request

app = application = Flask("datadoghq")
monitoring.monitor_flask(app)
handler = logging.StreamHandler(sys.stderr)
app.logger.addHandler(handler)
app.logger.setLevel("INFO")

record_dir = path.join(path.dirname(path.abspath(__file__)), "recorded")


def get_collection(name: str):
    c = pymongo.MongoClient("127.0.0.1", 27017, connectTimeoutMS=5000)
    db = c.get_database("datadog")
    return db.get_collection(name)


payload_names = [
    "check_run",
    "series",
    "intake",
    "logs",
]


def reset_records():
    for elt in payload_names:
        to_remove = path.join(record_dir, elt)
        if path.isfile(to_remove):
            app.logger.warning("rm %s", to_remove)
            os.remove(to_remove)

        try:
            get_collection(elt).drop()

        except Exception as e:
            app.logger.error(e)


def record_and_loads(filename: str, content_type: str, content_encoding: str, content: str):
    """
    :param filename:
    :param content_type:
    :param content_encoding:
    :param content:
    :return: list or dict
    """
    if content_type != "application/json":
        app.logger.error("Unsupported content-type: %s", content_type)
        raise TypeError(content_type)

    if content_encoding == "deflate":
        content = zlib.decompress(content)

    content = content.decode()
    content = f"{content}\n" if content[-1] != "\n" else content
    with open(path.join(record_dir, filename), "a") as f:
        f.write(content)

    return json.loads(content)


def patch_data(data, patch_key, patch_leaf):
    if isinstance(data, dict):
        return {patch_key(k): patch_data(v, patch_key, patch_leaf) for k, v in iter(data.items())}
    elif isinstance(data, list):
        return [patch_data(i, patch_key, patch_leaf) for i in data]
    else:
        return patch_leaf(data)


def fix_data(data):
    return patch_data(
        data,
        # Whereas dot (.) and dollar ($) are valid characters inside a JSON dict key,
        # they are not allowed as keys in a MongoDB BSON object.
        # The official MongoDB documentation suggests to replace them with their
        # unicode full width equivalent:
        # https://docs.mongodb.com/v2.6/faq/developers/#dollar-sign-operator-escaping
        patch_key=lambda x: x.translate(str.maketrans('.$', '\uff0e\uff04')),
        # Values that cannot fit in a 64 bits integer must be represented as a float.
        patch_leaf=lambda x: float(x) if isinstance(x, int) and x > 2**63 - 1 else x,
    )


def insert_series(data: dict):
    coll = get_collection("series")
    coll.insert_many(data["series"])


def insert_intake(data: dict):
    coll = get_collection("intake")
    coll.insert_one(data)


def insert_check_run(data: list):
    coll = get_collection("check_run")
    coll.insert_many(data)


def insert_logs(data: list):
    coll = get_collection("logs")
    coll.insert_many(data)


def get_series_from_query(q: dict):
    app.logger.info("Query is %s", q["query"])
    query = q["query"].replace("avg:", "")
    first_open_brace, first_close_brace = query.index("{"), query.index("}")

    metric_name = query[:first_open_brace]
    from_ts, to_ts = int(q["from"]), int(q["to"])

    # tags
    all_tags = query[first_open_brace + 1 : first_close_brace]
    all_tags = all_tags.split(",") if all_tags else []

    # group by
    # TODO
    last_open_brace, last_close_brace = query.rindex("{"), query.rindex("}")
    group_by = query[last_open_brace + 1 : last_close_brace].split(",")  # noqa: F841

    match_conditions = [
        {"metric": metric_name},
        {"points.0.0": {"$gt": from_ts}},
        {"points.0.0": {"$lt": to_ts}},
    ]
    if all_tags:
        match_conditions.append({'tags': {"$all": all_tags}})

    c = get_collection("series")
    aggregate = [
        {"$match": {"$and": match_conditions}},
        {"$unwind": "$points"},
        {"$group": {"_id": "$metric", "points": {"$push": "$points"}}},
        {"$sort": {"points.0": 1}},
    ]
    app.logger.info("Mongodb aggregate is %s", aggregate)
    cur = c.aggregate(aggregate)
    points_list = []
    for elt in cur:
        for p in elt["points"]:
            p[0] *= 1000
            points_list.append(p)

    result = {
        "status": "ok",
        "res_type": "time_series",
        "series": [
            {
                "metric": metric_name,
                "attributes": {},
                "display_name": metric_name,
                "unit": None,
                "pointlist": points_list,
                "end": points_list[-1][0] if points_list else 0.0,
                "interval": 600,
                "start": points_list[0][0] if points_list else 0.0,
                "length": len(points_list),
                "aggr": None,
                "scope": "host:vagrant-ubuntu-trusty-64",  # TODO
                "expression": query,
            }
        ],
        "from_date": from_ts,
        "group_by": ["host"],
        "to_date": to_ts,
        "query": q["query"],
        "message": "",
    }
    return result


@app.route("/api/v1/validate", methods=["GET"])
def validate():
    return Response(status=200)


@app.route("/api/v1/query", methods=["GET"])
def metrics_query():
    """
    Honor a query like documented here:
    https://docs.datadoghq.com/api/?lang=bash#query-time-series-points
    :return:
    """
    if "query" not in request.args or "from" not in request.args or "to" not in request.args:
        return Response(status=400)

    return jsonify(get_series_from_query(request.args))


@app.route("/api/v1/series", methods=["POST"])
def series():
    data = record_and_loads(
        filename="series",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data,
    )
    data = fix_data(data)
    insert_series(data)
    return Response(status=200)


@app.route("/api/v1/check_run", methods=["POST"])
def check_run():
    data = record_and_loads(
        filename="check_run",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data,
    )
    data = fix_data(data)
    insert_check_run(data)
    return Response(status=200)


@app.route("/intake/", methods=["POST"])
def intake():
    data = record_and_loads(
        filename="intake",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data,
    )
    data = fix_data(data)
    insert_intake(data)
    return Response(status=200)


@app.route("/v1/input/", methods=["POST"])
def logs():
    data = record_and_loads(
        filename="logs",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data,
    )
    data = fix_data(data)
    insert_logs(data)
    return Response(status=200)


@app.route("/api/v2/orch", methods=["POST"])
def orchestrator():
    # TODO
    return Response(status=200)


@app.before_request
def logging():
    # use only if you need to check headers
    # mind where the logs of this container go since headers contain an API key
    # app.logger.info(
    #     "path: %s, method: %s, content-type: %s, content-encoding: %s, content-length: %s, headers: %s",
    #     request.path, request.method, request.content_type, request.content_encoding, request.content_length, request.headers)
    app.logger.info(
        "path: %s, method: %s, content-type: %s, content-encoding: %s, content-length: %s",
        request.path,
        request.method,
        request.content_type,
        request.content_encoding,
        request.content_length,
    )


def stat_records():
    j = dict()
    for elt in payload_names:
        try:
            p = path.join(record_dir, elt)
            st = os.stat(p)
            lines = 0
            with open(p) as f:
                for _ in f:
                    lines += 1
            j[elt] = {"size": st.st_size, "lines": lines}

        except FileNotFoundError:
            j[elt] = {"size": -1, "lines": -1}
    return j


@app.route("/_/records")
def available_records():
    return jsonify(stat_records())


@app.route("/_/records/<string:name>")
def get_records(name):
    if name not in payload_names:
        return Response(status=404)

    if path.isfile(path.join(record_dir, name)) is False:
        return Response(status=503)

    payloads = list()
    with open(path.join(record_dir, name)) as f:
        for l in f:
            payloads.append(json.loads(l))
    return json.dumps(payloads), 200


@application.route('/', methods=['GET'])
def api_mapper():
    rules = [k.rule for k in application.url_map.iter_rules()]
    rules = list(set(rules))
    rules.sort()
    return jsonify(rules)


@application.route('/_/reset', methods=['POST'])
def reset():
    reset_records()
    return jsonify(stat_records())


@application.errorhandler(404)
def not_found(_):
    app.logger.warning("404 %s %s", request.path, request.method)
    return Response("404", status=404, mimetype="text/plain")


if __name__ == '__main__':
    app.run(host="0.0.0.0", debug=True, port=5000)
