"""SAP ABAP (SAPControl) custom Agent check.

Scrapes the sapstartsrv **SAPControl** SOAP web service exposed by an SAP ABAP
system (here the ABAP Platform Trial container, SID A4H, instance 00) and emits
work-process, process-state, dispatcher-queue and enqueue-lock metrics.

SOAP contract (verified):
  * Transport : HTTP. Instance NN exposes SAPControl on port 5<NN>13
                (instance 00 -> 50013). WSDL at <url>/?wsdl.
  * Style     : SOAP 1.1, document/literal. POST to ``<sapcontrol_url>/``.
  * Headers   : ``Content-Type: text/xml; charset=UTF-8`` and an EMPTY
                ``SOAPAction: ""`` header. Namespace ``urn:SAPControl``.
  * Envelope  : <soap:Envelope><soap:Body><urn:<Method>/></soap:Body></...>
                (parameter-less read methods take an empty element).
  * Response  : <...Body><urn:<Method>Response><item>...</item>...>. Repeated
                struct rows appear as repeated ``<item>`` elements. We parse
                with xml.etree, matching by local tag name (namespace-agnostic).

Authentication:
  SAPControl methods are split protected/unprotected via the profile parameter
  ``service/protectedwebmethods``. Read methods are commonly UNPROTECTED (no
  creds). When protected, auth is HTTP Basic as the OS user a4hadm (<sid>adm) --
  set ``username``/``password`` in conf.yaml (sourced from the container env).
  There is no API key / token endpoint; credentials are OS-user Basic auth only.
  To unprotect instead, add the read methods to service/protectedwebmethods in
  the instance profile.

STATE_COLOR enum (SAPControl dispstatus / gray-green-yellow-red):
  SAPControl-GRAY=1, SAPControl-GREEN=2, SAPControl-YELLOW=3, SAPControl-RED=4.
  Emitted as an integer gauge so dashboards/monitors can alert on >2.

Emitted metrics (all gauges unless noted); names lowercased, non-alnum -> "_":
  GetProcessList
    * sap.sapcontrol.process.status  (STATE_COLOR)   tags process:<name>,pid:<pid>
    * sap.sapcontrol.process.count   (per dispstatus) tag  status:<color>
  GetSystemInstanceList
    * sap.sapcontrol.instance.status (STATE_COLOR)   tags instance_hostname:<h>,
                                                          features:<f>,instance_nr:<n>
  ABAPGetWPTable
    * sap.abap.workprocess.count     (per Typ x Status) tags type:<typ>,status:<st>
    * sap.abap.workprocess.cpu       (per WP, best-effort) tags type,wp_no,pid
    * sap.abap.workprocess.time      (per WP, best-effort) tags type,wp_no,pid
  GetQueueStatistic
    * sap.abap.queue.now/high/max/writes/reads          tag  queue:<Typ>
  EnqGetStatistic
    * sap.abap.enqueue.locks_now/locks_high/locks_max/
      enqueue_requests/enqueue_rejects/enqueue_errors   (best-effort, singleton)
  Service check
    * sap.sapcontrol.can_connect  OK/CRITICAL           tag sapcontrol_url:<url>

The set of methods called is config-driven (``methods:`` list); per-method
errors (transport, non-200, SOAP fault, parse) are logged and skipped so one bad
method never kills the rest.

Reference metric model: github.com/SUSE/sap_host_exporter (lib/sapcontrol:
sap_start_service_*, sap_dispatcher_queue_*, sap_enqueue_server_*).
"""

import re
import xml.etree.ElementTree as ET

from datadog_checks.base import AgentCheck

try:
    import requests
except ImportError:
    requests = None

DEFAULT_SAPCONTROL_URL = "http://172.17.0.1:50013"
SOAP_NS = "urn:SAPControl"

SERVICE_CHECK = "sap.sapcontrol.can_connect"
HTTP_TIMEOUT = 30

DEFAULT_METHODS = [
    "GetProcessList",
    "GetSystemInstanceList",
    "ABAPGetWPTable",
    "GetQueueStatistic",
    "EnqGetStatistic",
]

# SAPControl STATE_COLOR string -> integer enum.
STATE_COLOR = {
    "SAPControl-GRAY": 1,
    "SAPControl-GREEN": 2,
    "SAPControl-YELLOW": 3,
    "SAPControl-RED": 4,
}

_METRIC_SANITIZE = re.compile(r"[^0-9a-z]+")

# Leading number optionally followed by a unit token, e.g. "47 %", "5354 MB",
# "5402 /S", "2902.26". Group 1 = number, group 2 = trailing unit (may be empty).
_ALERT_VALUE = re.compile(r"^\s*(-?\d+(?:\.\d+)?)\s*(\S+)?\s*$")

# Cap AlertTree processing to bound tag cardinality.
MAX_ALERT_ITEMS = 500


def _sanitize(text):
    """Lowercase and collapse runs of non-alphanumerics into a single underscore."""
    return _METRIC_SANITIZE.sub("_", (text or "").lower()).strip("_")


def _local(tag):
    """Strip any XML namespace, returning the local element name."""
    return tag.rsplit("}", 1)[-1] if "}" in tag else tag


def _state_color(value):
    """Map a SAPControl-<COLOR> string to its integer enum, else None."""
    if value is None:
        return None
    return STATE_COLOR.get(value.strip())


def _to_float(value):
    try:
        return float(str(value).strip())
    except (TypeError, ValueError):
        return None


class SapAbapCheck(AgentCheck):
    def check(self, instance):
        url = (instance.get("sapcontrol_url") or DEFAULT_SAPCONTROL_URL).rstrip("/")
        methods = instance.get("methods") or DEFAULT_METHODS
        username = instance.get("username")
        password = instance.get("password")

        base_tags = list(instance.get("tags") or [])
        base_tags.append(f"sapcontrol_url:{url}")

        if requests is None:
            self.log.error("sap_abap: 'requests' library unavailable")
            self.service_check(
                SERVICE_CHECK,
                AgentCheck.CRITICAL,
                tags=base_tags,
                message="requests unavailable",
            )
            return

        auth = (username, password) if (username and password) else None

        # can_connect gate: probe once with GetProcessList. A transport error or
        # non-200 is fatal for the whole run (report CRITICAL); a SOAP fault on a
        # single method is per-method and handled below.
        probe = self._call(url, "GetProcessList", auth)
        if probe is None:
            self.service_check(
                SERVICE_CHECK,
                AgentCheck.CRITICAL,
                tags=base_tags,
                message=f"cannot reach SAPControl at {url}",
            )
            return
        self.service_check(SERVICE_CHECK, AgentCheck.OK, tags=base_tags)

        handlers = {
            "GetProcessList": self._handle_process_list,
            "GetSystemInstanceList": self._handle_instance_list,
            "ABAPGetWPTable": self._handle_wp_table,
            "GetQueueStatistic": self._handle_queue_statistic,
            "EnqGetStatistic": self._handle_enq_statistic,
            "GetAlertTree": self._handle_alert_tree,
        }

        for method in methods:
            handler = handlers.get(method)
            if handler is None:
                self.log.warning("sap_abap: no handler for method %s; skipping", method)
                continue
            root = probe if method == "GetProcessList" else self._call(url, method, auth)
            if root is None:
                self.log.warning("sap_abap: method %s returned no usable response", method)
                continue
            try:
                handler(root, base_tags)
            except Exception as exc:  # noqa: BLE001 - one bad payload must not kill the rest
                self.log.error("sap_abap: failed to parse %s response: %s", method, exc)

    # ------------------------------------------------------------------ SOAP

    def _call(self, url, method, auth):
        """POST a parameter-less SAPControl read method; return the parsed XML root or None."""
        envelope = (
            '<?xml version="1.0" encoding="UTF-8"?>'
            '<SOAP-ENV:Envelope '
            'xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" '
            f'xmlns:urn="{SOAP_NS}">'
            f"<SOAP-ENV:Body><urn:{method}/></SOAP-ENV:Body>"
            "</SOAP-ENV:Envelope>"
        )
        headers = {"Content-Type": "text/xml; charset=UTF-8", "SOAPAction": '""'}
        try:
            resp = requests.post(
                url + "/",
                data=envelope.encode("utf-8"),
                headers=headers,
                auth=auth,
                timeout=HTTP_TIMEOUT,
            )
        except Exception as exc:  # noqa: BLE001
            self.log.error("sap_abap: %s request to %s failed: %s", method, url, exc)
            return None

        if resp.status_code == 401:
            self.log.error(
                "sap_abap: %s returned HTTP 401 (method protected); set username/password "
                "(a4hadm) or unprotect it via service/protectedwebmethods",
                method,
            )
            return None
        if resp.status_code != 200:
            self.log.error(
                "sap_abap: %s returned HTTP %s: %s",
                method,
                resp.status_code,
                resp.text[:200],
            )
            return None

        try:
            return ET.fromstring(resp.content)
        except ET.ParseError as exc:
            self.log.error("sap_abap: %s response is not valid XML: %s", method, exc)
            return None

    @staticmethod
    def _items(root):
        """Yield the repeated <item> struct rows from a SAPControl response body.

        The response is <Body><MethodResponse><list-or-tree><item>...<item>...>.
        Namespace-agnostic: match on local tag name 'item'.
        """
        return [el for el in root.iter() if _local(el.tag) == "item"]

    @staticmethod
    def _fields(item):
        """Return a dict of {localFieldName: text} for one struct <item>."""
        out = {}
        for child in list(item):
            out[_local(child.tag)] = (child.text or "").strip()
        return out

    # -------------------------------------------------------------- handlers

    def _handle_process_list(self, root, base_tags):
        counts = {}
        for item in self._items(root):
            f = self._fields(item)
            name = f.get("name") or "unknown"
            pid = f.get("pid") or ""
            color = f.get("dispstatus")
            enum = _state_color(color)
            if enum is not None:
                self.gauge(
                    "sap.sapcontrol.process.status",
                    enum,
                    tags=base_tags + [f"process:{name}", f"pid:{pid}"],
                )
            key = color or "unknown"
            counts[key] = counts.get(key, 0) + 1
        for color, n in counts.items():
            self.gauge(
                "sap.sapcontrol.process.count",
                n,
                tags=base_tags + [f"status:{color}"],
            )

    def _handle_instance_list(self, root, base_tags):
        for item in self._items(root):
            f = self._fields(item)
            enum = _state_color(f.get("dispstatus"))
            if enum is None:
                continue
            tags = base_tags + [
                "instance_hostname:{}".format(f.get("hostname") or "unknown"),
                "features:{}".format(f.get("features") or "unknown"),
                "instance_nr:{}".format(f.get("instanceNr") or ""),
            ]
            self.gauge("sap.sapcontrol.instance.status", enum, tags=tags)

    def _handle_wp_table(self, root, base_tags):
        combo_counts = {}
        for item in self._items(root):
            f = self._fields(item)
            wp_typ = f.get("Typ") or "unknown"
            wp_status = f.get("Status") or "unknown"
            wp_no = f.get("No") or ""
            pid = f.get("Pid") or ""

            combo = (wp_typ, wp_status)
            combo_counts[combo] = combo_counts.get(combo, 0) + 1

            per_wp_tags = base_tags + [
                f"type:{wp_typ}",
                f"wp_no:{wp_no}",
                f"pid:{pid}",
            ]
            cpu = _to_float(f.get("Cpu"))
            if cpu is not None:
                self.gauge("sap.abap.workprocess.cpu", cpu, tags=per_wp_tags)
            t = _to_float(f.get("Time"))
            if t is not None:
                self.gauge("sap.abap.workprocess.time", t, tags=per_wp_tags)

        for (wp_typ, wp_status), n in combo_counts.items():
            self.gauge(
                "sap.abap.workprocess.count",
                n,
                tags=base_tags + [f"type:{wp_typ}", f"status:{wp_status}"],
            )

    def _handle_queue_statistic(self, root, base_tags):
        # Field names per SAPControl GetQueueStatistic: Typ, Now, High, Max, Writes, Reads.
        field_to_metric = {
            "Now": "sap.abap.queue.now",
            "High": "sap.abap.queue.high",
            "Max": "sap.abap.queue.max",
            "Writes": "sap.abap.queue.writes",
            "Reads": "sap.abap.queue.reads",
        }
        for item in self._items(root):
            f = self._fields(item)
            qtags = base_tags + ["queue:{}".format(f.get("Typ") or "unknown")]
            for field, metric in field_to_metric.items():
                value = _to_float(f.get(field))
                if value is not None:
                    self.gauge(metric, value, tags=qtags)

    def _handle_enq_statistic(self, root, base_tags):
        # EnqGetStatistic returns a single <...:EnqStatistic> struct (NOT an
        # <item> list). Locate it by local tag name (namespace-agnostic) and
        # iterate its direct children. Children ending in '-state' carry a
        # SAPControl-<COLOR> value; every other child is numeric. -1 is SAP's
        # "not available" sentinel and is skipped.
        enq = None
        for el in root.iter():
            if _local(el.tag) == "EnqStatistic":
                enq = el
                break
        if enq is None:
            self.log.warning("sap_abap: EnqGetStatistic response has no EnqStatistic element")
            return
        for child in list(enq):
            try:
                local = _local(child.tag)
                metric = "sap.abap.enqueue.{}".format(local.replace("-", "_"))
                text = (child.text or "").strip()
                if local.endswith("-state"):
                    enum = _state_color(text)
                    if enum is not None:
                        self.gauge(metric, enum, tags=base_tags)
                    continue
                value = _to_float(text)
                if value is None or value == -1:
                    continue
                self.gauge(metric, value, tags=base_tags)
            except Exception as exc:  # noqa: BLE001 - skip one bad field, keep the rest
                self.log.debug("sap_abap: skipping EnqStatistic field: %s", exc)

    def _handle_alert_tree(self, root, base_tags):
        # GetAlertTree returns <tree><item>...</item>...>. Each item has a CCMS
        # node <name> and an <ActualValue> that is EITHER a SAPControl-<COLOR>
        # status string or a numeric-with-unit string ("47 %", "5354 MB",
        # "5402 /S", "2902.26"). Cap items to bound tag cardinality.
        items = self._items(root)
        if len(items) > MAX_ALERT_ITEMS:
            self.log.debug(
                "sap_abap: GetAlertTree returned %d items; processing first %d",
                len(items),
                MAX_ALERT_ITEMS,
            )
        for item in items[:MAX_ALERT_ITEMS]:
            try:
                f = self._fields(item)
                name = f.get("name") or "unknown"
                actual = (f.get("ActualValue") or "").strip()
                if not actual:
                    continue
                if actual.startswith("SAPControl-"):
                    enum = _state_color(actual)
                    if enum is not None:
                        self.gauge(
                            "sap.abap.alert.status",
                            enum,
                            tags=base_tags + [f"alert:{name}"],
                        )
                    continue
                m = _ALERT_VALUE.match(actual)
                if m is None:
                    continue
                value = _to_float(m.group(1))
                if value is None:
                    continue
                tags = base_tags + [f"alert:{name}"]
                unit = m.group(2)
                if unit:
                    tags.append(f"unit:{unit}")
                self.gauge("sap.abap.alert.value", value, tags=tags)
            except Exception as exc:  # noqa: BLE001 - skip one bad item, keep the rest
                self.log.debug("sap_abap: skipping AlertTree item: %s", exc)
