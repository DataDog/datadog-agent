import logging
import os
import sys
import ujson as json
import zlib
from os import path

import pymongo
from flask import Flask, request, Response, jsonify

import monitoring

app = application = Flask("datadoghq")
monitoring.monitor_flask(app)
handler = logging.StreamHandler(sys.stderr)
app.logger.addHandler(handler)
app.logger.setLevel("INFO")

record_dir = path.join(path.dirname(path.dirname(path.abspath(__file__))), "recorded")


def get_collection(name: str):
    c = pymongo.MongoClient("127.0.0.1", 27017, connectTimeoutMS=5000)
    db = c.get_database("datadog")
    return db.get_collection(name)


payload_names = [
    "check_run",
    "series",
    "intake",
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
    content = "%s\n" % content if content[-1] != "\n" else content
    with open(path.join(record_dir, filename), "a") as f:
        f.write(content)

    return json.loads(content)


def insert_series(data: dict):
    coll = get_collection("series")
    coll.insert_many(data["series"])


def insert_intake(data: dict):
    coll = get_collection("intake")
    coll.insert(data)


def insert_check_run(data: list):
    coll = get_collection("check_run")
    coll.insert_many(data)


@app.route("/api/v1/validate", methods=["GET"])
def validate():
    return Response(status=200)


@app.route("/api/v1/series", methods=["POST"])
def series():
    data = record_and_loads(
        filename="series",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data
    )
    insert_series(data)
    return Response(status=200)


@app.route("/api/v1/check_run", methods=["POST"])
def check_run():
    data = record_and_loads(
        filename="check_run",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data
    )
    insert_check_run(data)
    return Response(status=200)


@app.route("/intake/", methods=["POST"])
def intake():
    data = record_and_loads(
        filename="intake",
        content_type=request.content_type,
        content_encoding=request.content_encoding,
        content=request.data
    )
    insert_intake(data)
    return Response(status=200)


@app.before_request
def logging():
    app.logger.info(
        "path: %s, method: %s, content-type: %s, content-encoding: %s, content-length: %s",
        request.path, request.method, request.content_type, request.content_encoding, request.content_length)


def stat_records():
    j = dict()
    for elt in payload_names:
        try:
            p = path.join(record_dir, elt)
            st = os.stat(p)
            lines = 0
            with open(p, 'r') as f:
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
    with open(path.join(record_dir, name), 'r') as f:
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
    app.run(debug=True, port=5000)
