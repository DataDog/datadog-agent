"""SAP S/4HANA Order-to-Cash (O2C) crawler.

Emits:
  * O2C KPI + failure/block metrics (gauges),
  * per-stage lead-time histograms,
  * per-order PROCESS-FLOW records as structured JSON logs -- built DYNAMICALLY
    from SAP's own document flow (no hardcoded step sequence), so the steps adapt
    to whatever the customer configured. Each record has per-item step graphs +
    an order-level roll-up (UI-ready stepper model).

Dynamic flow source (validated): the sales-order ITEM navigation
  A_SalesOrder(so)/to_Item?$expand=to_SubsequentProcFlowDocItem
returns, per item, the actual subsequent documents with:
  SubsequentDocumentCategory (SAP-standard doc category = step type),
  ProcessFlowLevel + predecessor (sequence), CreationDate (when),
  RelatedProcFlowDocStsFieldName -> the relevant status field (state).
Custom document *types* still roll up to standard *categories*, so labels are
config-agnostic. Dates arrive as /Date(epoch_ms)/ (day-granular).
"""

import datetime
import json
import os
import re
from collections import Counter, defaultdict

import requests
from datadog_checks.base import AgentCheck

_DATE_RE = re.compile(r"(\d+)")

# SAP SDDocumentCategory (VBTYP) code -> human label. Config-independent.
_CAT = {
    "A": "Inquiry",
    "B": "Quotation",
    "C": "Order",
    "D": "Item Proposal",
    "E": "Scheduling Agreement",
    "F": "Contract",
    "G": "Contract",
    "H": "Returns",
    "I": "Order w/o Charge",
    "J": "Delivery",
    "K": "Credit Memo Req",
    "L": "Debit Memo Req",
    "M": "Invoice",
    "N": "Invoice Cancellation",
    "O": "Credit Memo",
    "P": "Debit Memo",
    "R": "Goods Movement",
    "S": "Credit Memo Cancel",
    "T": "Returns Delivery",
    "U": "Pro Forma Invoice",
    "6": "Contract",
    "7": "Return Request",
}


def _cat_label(code):
    return _CAT.get(code or "", "Doc(%s)" % (code or "?"))


class SapS4hanaO2cCheck(AgentCheck):
    def check(self, instance):
        base = instance["base_url"].rstrip("/")
        client = str(instance.get("sap_client", "100"))
        user = instance.get("username")
        pw = instance.get("password")
        auth = (user, pw) if user and pw else None
        max_records = int(instance.get("max_records", 2000))
        join_sample = int(instance.get("join_sample", 40))
        timeout = float(instance.get("timeout", 30))
        log_file = instance.get("log_file", "/var/log/sap-o2c/o2c.jsonl")
        tags = list(instance.get("tags") or [])
        tags.append(f"sap_client:{client}")
        sid = next((t.split(":", 1)[1] for t in tags if t.startswith("sap_sid:")), None)

        sess = requests.Session()
        sess.headers.update({"Accept": "application/json"})
        ok = True

        def fetch(path, top=None, skip=None, expand=None):
            params = {"sap-client": client, "$format": "json"}
            if top is not None:
                params["$top"] = top
            if skip is not None:
                params["$skip"] = skip
            if expand:
                params["$expand"] = expand
            r = sess.get(base + "/sap/opu/odata/sap/" + path, params=params, auth=auth, timeout=timeout)
            r.raise_for_status()
            d = r.json()["d"]
            return d.get("results", [d]) if isinstance(d, dict) else []

        def page_all(path):
            out, skip, size = [], 0, 500
            while len(out) < max_records:
                batch = fetch(path, top=min(size, max_records - len(out)), skip=skip)
                if not batch:
                    break
                out.extend(batch)
                if len(batch) < size:
                    break
                skip += len(batch)
            return out

        def as_date(v):
            m = _DATE_RE.search(v or "")
            return datetime.date.fromtimestamp(int(m.group(1)) / 1000) if m else None

        def iso(d):
            return d.isoformat() if d else None

        today = datetime.date.today()

        # ---- Sales Orders: counts, backlog, rejections, amount ----
        orders = []
        try:
            orders = page_all("API_SALES_ORDER_SRV/A_SalesOrder")
            combo, open_by_org, rejected_by_org = Counter(), Counter(), Counter()
            amt = defaultdict(float)
            for r in orders:
                org = r.get("SalesOrganization") or "unknown"
                sd = r.get("OverallSDProcessStatus") or "none"
                ot = r.get("SalesOrderType") or "none"
                cur = r.get("TransactionCurrency") or "none"
                combo[(org, sd, ot)] += 1
                if sd != "C":
                    open_by_org[org] += 1
                if (r.get("OverallSDDocumentRejectionSts") or "") in ("B", "C"):
                    rejected_by_org[org] += 1
                try:
                    amt[(org, cur)] += float(r.get("TotalNetAmount") or 0)
                except (TypeError, ValueError):
                    pass
            self.gauge("sap.s4hana.o2c.sales_orders.total", len(orders), tags=tags)
            for (org, sd, ot), c in combo.items():
                self.gauge(
                    "sap.s4hana.o2c.sales_orders.count",
                    c,
                    tags=tags + ["sales_org:%s" % org, "sd_status:%s" % sd, "order_type:%s" % ot],
                )
            for org, c in open_by_org.items():
                self.gauge("sap.s4hana.o2c.open_sales_documents", c, tags=tags + ["sales_org:%s" % org])
            for org, c in rejected_by_org.items():
                self.gauge("sap.s4hana.o2c.orders_rejected", c, tags=tags + ["sales_org:%s" % org])
            for (org, cur), a in amt.items():
                self.gauge(
                    "sap.s4hana.o2c.sales_order_net_amount", a, tags=tags + ["sales_org:%s" % org, "currency:%s" % cur]
                )
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_o2c: sales orders failed: %s", exc)

        # ---- Deliveries: unbilled, overdue-PGI, incomplete, not-picked ----
        try:
            dels = page_all("API_OUTBOUND_DELIVERY_SRV/A_OutbDeliveryHeader")
            unbilled = overdue = incomplete = not_picked = 0
            incompl_fields = [
                "HdrGeneralIncompletionStatus",
                "HdrGoodsMvtIncompletionStatus",
                "HeaderBillgIncompletionStatus",
                "HeaderDelivIncompletionStatus",
                "HeaderPackingIncompletionSts",
                "HeaderPickgIncompletionStatus",
            ]
            for r in dels:
                gm = r.get("OverallGoodsMovementStatus") or ""
                if gm == "C" and (r.get("OverallDelivReltdBillgStatus") or "") != "C":
                    unbilled += 1
                pgi = as_date(r.get("PlannedGoodsIssueDate"))
                if pgi and pgi < today and gm != "C":
                    overdue += 1
                if any((r.get(f) or "C") not in ("C", "") for f in incompl_fields):
                    incomplete += 1
                if (r.get("OverallPickingStatus") or "C") not in ("C", ""):
                    not_picked += 1
            self.gauge("sap.s4hana.o2c.deliveries.total", len(dels), tags=tags)
            self.gauge("sap.s4hana.o2c.unbilled_deliveries", unbilled, tags=tags)
            self.gauge("sap.s4hana.o2c.deliveries_overdue_pgi", overdue, tags=tags)
            self.gauge("sap.s4hana.o2c.deliveries_incomplete", incomplete, tags=tags)
            self.gauge("sap.s4hana.o2c.deliveries_not_picked", not_picked, tags=tags)
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_o2c: deliveries failed: %s", exc)

        # ---- Billing: not-posted, transfer-failed, cancelled, amount ----
        try:
            bills = page_all("API_BILLING_DOCUMENT_SRV/A_BillingDocument")
            notposted = Counter()
            transfer_failed = cancelled = 0
            billamt = defaultdict(float)
            for r in bills:
                if (r.get("AccountingPostingStatus") or "none") != "C":
                    notposted[r.get("CompanyCode") or "unknown"] += 1
                if (r.get("AccountingTransferStatus") or "") not in ("C", ""):
                    transfer_failed += 1
                if r.get("BillingDocumentIsCancelled") in (True, "true", "X"):
                    cancelled += 1
                try:
                    billamt[(r.get("CompanyCode") or "unknown", r.get("TransactionCurrency") or "none")] += float(
                        r.get("TotalNetAmount") or 0
                    )
                except (TypeError, ValueError):
                    pass
            self.gauge("sap.s4hana.o2c.billing_docs.total", len(bills), tags=tags)
            self.gauge("sap.s4hana.o2c.billing_transfer_failed", transfer_failed, tags=tags)
            self.gauge("sap.s4hana.o2c.billing_cancelled", cancelled, tags=tags)
            for cc, c in notposted.items():
                self.gauge("sap.s4hana.o2c.billing_not_posted", c, tags=tags + ["company_code:%s" % cc])
            for (cc, cur), a in billamt.items():
                self.gauge(
                    "sap.s4hana.o2c.billed_net_amount", a, tags=tags + ["company_code:%s" % cc, "currency:%s" % cur]
                )
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_o2c: billing failed: %s", exc)

        # ---- Dynamic per-order document-flow -> stepper logs ----
        def node_state(node):
            fld = node.get("RelatedProcFlowDocStsFieldName")
            val = (node.get(fld) if fld else None) or node.get("SDProcessStatus") or ""
            if val == "C":
                return "complete"
            return "in_progress"  # doc exists in the flow -> step reached but not done

        logs = []
        stuck = 0
        sample = orders[:join_sample]
        for soh in sample:
            so = soh.get("SalesOrder")
            if not so:
                continue
            try:
                items = fetch(
                    "API_SALES_ORDER_SRV/A_SalesOrder('%s')/to_Item" % so,
                    expand="to_SubsequentProcFlowDocItem,to_ScheduleLine",
                )
            except Exception as exc:  # noqa: BLE001
                self.log.warning("sap_s4hana_o2c: flow fetch failed for SO %s: %s", so, exc)
                continue
            order_date = as_date(soh.get("CreationDate"))
            org = soh.get("SalesOrganization") or "unknown"
            item_records = []
            order_backordered = False
            # roll-up accumulators: category -> [reached_items, complete_items]
            cat_first_level = {"C": -1}
            cat_reached = Counter()
            cat_complete = Counter()
            lead = {}  # category -> earliest date (order-level lead-time inputs)
            lead["C"] = order_date
            for it in items:
                iid = it.get("SalesOrderItem")
                plant = it.get("ProductionPlant") or it.get("Plant")
                # ATP / backorder: schedule-line confirmed-by-availability < ordered qty
                sched = (
                    (it.get("to_ScheduleLine") or {}).get("results", [])
                    if isinstance(it.get("to_ScheduleLine"), dict)
                    else []
                )
                ordered = confirmed = 0.0
                for sl in sched:
                    try:
                        ordered += float(sl.get("ScheduleLineOrderQuantity") or 0)
                        confirmed += float(sl.get("ConfdOrderQtyByMatlAvailCheck") or 0)
                    except (TypeError, ValueError):
                        pass
                item_backordered = ordered > 0 and confirmed < ordered
                if item_backordered:
                    order_backordered = True
                flow = (
                    (it.get("to_SubsequentProcFlowDocItem") or {}).get("results", [])
                    if isinstance(it.get("to_SubsequentProcFlowDocItem"), dict)
                    else []
                )
                # root step = the order line itself
                steps = [
                    {
                        "seq": -1,
                        "category": "C",
                        "label": "Order",
                        "doc": so,
                        "state": "complete",
                        "at": iso(order_date),
                    }
                ]
                seen_cats = {"C"}
                for nd in sorted(
                    flow, key=lambda n: (int(n.get("ProcessFlowLevel") or 0), n.get("CreationDate") or "")
                ):
                    cat = nd.get("SubsequentDocumentCategory")
                    lvl = int(nd.get("ProcessFlowLevel") or 0)
                    st = node_state(nd)
                    at = as_date(nd.get("CreationDate"))
                    steps.append(
                        {
                            "seq": lvl,
                            "category": cat,
                            "label": _cat_label(cat),
                            "doc": nd.get("SubsequentDocument"),
                            "state": st,
                            "at": iso(at),
                        }
                    )
                    seen_cats.add(cat)
                    cat_first_level.setdefault(cat, lvl)
                    cat_first_level[cat] = min(cat_first_level[cat], lvl)
                    if cat not in lead or (at and (lead.get(cat) is None or at < lead[cat])):
                        lead[cat] = at
                # per-item roll-up contributions
                for cat in seen_cats:
                    cat_reached[cat] += 1
                    node_complete = (cat == "C") or all(s["state"] == "complete" for s in steps if s["category"] == cat)
                    if node_complete:
                        cat_complete[cat] += 1
                item_flow = "complete" if all(s["state"] == "complete" for s in steps) else "in_progress"
                item_records.append(
                    {
                        "item": iid,
                        "material": it.get("Material"),
                        "plant": plant,
                        "backordered": item_backordered,
                        "flow_status": item_flow,
                        "steps": steps,
                    }
                )

            n_items = max(len(items), 1)
            # order-level summary: one entry per category, in flow order
            summary = []
            for cat in sorted(cat_first_level, key=lambda c: cat_first_level[c]):
                comp, reached = cat_complete[cat], cat_reached[cat] if cat != "C" else n_items
                comp = comp if cat != "C" else n_items
                if comp >= n_items:
                    state = "complete"
                elif reached > 0:
                    state = "partial"
                else:
                    state = "pending"
                summary.append(
                    {
                        "category": cat,
                        "label": _cat_label(cat),
                        "state": state,
                        "items_complete": comp,
                        "items_total": n_items,
                    }
                )
            current = next((s["label"] for s in summary if s["state"] != "complete"), None)
            flow_status = "complete" if all(s["state"] == "complete" for s in summary) else "in_progress"
            if flow_status != "complete":
                stuck += 1

            # order-level lead-time histograms (order -> delivery(J) -> invoice(M))
            lt_tags = tags + ["sales_org:%s" % org]
            if lead.get("C") and lead.get("J"):
                self.histogram("sap.s4hana.o2c.lead_time.order_to_delivery", (lead["J"] - lead["C"]).days, tags=lt_tags)
            if lead.get("J") and lead.get("M"):
                self.histogram(
                    "sap.s4hana.o2c.lead_time.delivery_to_invoice", (lead["M"] - lead["J"]).days, tags=lt_tags
                )
            if lead.get("C") and lead.get("M"):
                self.histogram("sap.s4hana.o2c.lead_time.order_to_invoice", (lead["M"] - lead["C"]).days, tags=lt_tags)

            plants = sorted({r["plant"] for r in item_records if r.get("plant")})
            logs.append(
                {
                    "ddsource": "sap_s4hana",
                    "service": "o2c",
                    "sap_sid": sid,
                    "sales_org": org,
                    "o2c_sales_order": so,
                    "created": iso(order_date),
                    "amount": soh.get("TotalNetAmount"),
                    "currency": soh.get("TransactionCurrency"),
                    "flow_status": flow_status,
                    "current_step": current,
                    "backordered": order_backordered,
                    "plants": plants,
                    "order_summary": summary,
                    "items": item_records,
                }
            )

        self.gauge("sap.s4hana.o2c.orders_traced", len(logs), tags=tags)
        self.gauge("sap.s4hana.o2c.orders_stuck", stuck, tags=tags)
        # backordered over the traced sample (ATP-unconfirmed schedule lines)
        self.gauge("sap.s4hana.o2c.orders_backordered_sampled", sum(1 for r in logs if r.get("backordered")), tags=tags)
        if logs:
            try:
                d = os.path.dirname(log_file)
                if d and not os.path.isdir(d):
                    os.makedirs(d)
                with open(log_file, "a") as fh:
                    for rec in logs:
                        fh.write(json.dumps(rec) + "\n")
            except Exception as exc:  # noqa: BLE001
                self.log.warning("sap_s4hana_o2c: could not write log file %s: %s", log_file, exc)

        self.service_check(
            "sap.s4hana.o2c.can_connect",
            AgentCheck.OK if ok else AgentCheck.CRITICAL,
            tags=tags + ["base_url:%s" % base],
        )
