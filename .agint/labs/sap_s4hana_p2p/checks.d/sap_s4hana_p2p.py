"""SAP S/4HANA Procure-to-Pay (P2P) crawler.

Sibling to the O2C crawler; same pattern, different process. Polls the read-only
A2X OData APIs for the P2P chain and emits KPI/failure metrics + per-PO flow logs.

Spine = Purchase Order. Chain: Purchase Requisition -> Purchase Order ->
Goods Receipt (material doc) -> Supplier Invoice (-> Payment, AR/AP not on this box).
Links: PO item -> PurchaseRequisition; supplier invoice header
SupplierInvoiceIDByInvcgParty -> PurchaseOrder.

Services (all active on the appliance):
  API_PURCHASEREQ_PROCESS_SRV/A_PurchaseRequisitionHeader
  API_PURCHASEORDER_PROCESS_SRV/A_PurchaseOrder
  API_MATERIAL_DOCUMENT_SRV/A_MaterialDocumentHeader
  API_SUPPLIERINVOICE_PROCESS_SRV/A_SupplierInvoice
"""

import datetime
import json
import os
import re
from collections import Counter, defaultdict

import requests
from datadog_checks.base import AgentCheck

_DATE_RE = re.compile(r"(\d+)")


class SapS4hanaP2pCheck(AgentCheck):
    def check(self, instance):
        base = instance["base_url"].rstrip("/")
        client = str(instance.get("sap_client", "100"))
        user = instance.get("username")
        pw = instance.get("password")
        auth = (user, pw) if user and pw else None
        max_records = int(instance.get("max_records", 2000))
        join_sample = int(instance.get("join_sample", 40))
        timeout = float(instance.get("timeout", 30))
        log_file = instance.get("log_file", "/var/log/sap-p2p/p2p.jsonl")
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

        # ---- Purchase Requisitions ----
        try:
            prs = page_all("API_PURCHASEREQ_PROCESS_SRV/A_PurchaseRequisitionHeader")
            self.gauge("sap.s4hana.p2p.purchase_requisitions.total", len(prs), tags=tags)
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_p2p: purchase requisitions failed: %s", exc)

        # ---- Purchase Orders ----
        pos = []
        try:
            pos = page_all("API_PURCHASEORDER_PROCESS_SRV/A_PurchaseOrder")
            combo = Counter()
            open_by_org = Counter()
            not_released = 0
            for r in pos:
                org = r.get("PurchasingOrganization") or "unknown"
                st = r.get("PurchasingProcessingStatus") or "none"
                ot = r.get("PurchaseOrderType") or "none"
                combo[(org, st, ot)] += 1
                if r.get("PurchasingCompletenessStatus") in (False, "false", ""):
                    open_by_org[org] += 1
                if r.get("ReleaseIsNotCompleted") in (True, "true", "X"):
                    not_released += 1
            self.gauge("sap.s4hana.p2p.purchase_orders.total", len(pos), tags=tags)
            for (org, st, ot), c in combo.items():
                self.gauge(
                    "sap.s4hana.p2p.purchase_orders.count",
                    c,
                    tags=tags + ["purchasing_org:%s" % org, "processing_status:%s" % st, "po_type:%s" % ot],
                )
            for org, c in open_by_org.items():
                self.gauge("sap.s4hana.p2p.purchase_orders_open", c, tags=tags + ["purchasing_org:%s" % org])
            self.gauge("sap.s4hana.p2p.purchase_orders_not_released", not_released, tags=tags)
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_p2p: purchase orders failed: %s", exc)

        # ---- Goods Receipts (material documents) ----
        try:
            mds = page_all("API_MATERIAL_DOCUMENT_SRV/A_MaterialDocumentHeader")
            by_code = Counter()
            for r in mds:
                by_code[r.get("GoodsMovementCode") or "none"] += 1
            self.gauge("sap.s4hana.p2p.material_documents.total", len(mds), tags=tags)
            for code, c in by_code.items():
                self.gauge("sap.s4hana.p2p.material_documents.count", c, tags=tags + ["goods_movement_code:%s" % code])
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_p2p: material documents failed: %s", exc)

        # ---- Supplier Invoices ----
        try:
            invs = page_all("API_SUPPLIERINVOICE_PROCESS_SRV/A_SupplierInvoice")
            by_status = Counter()
            amt = defaultdict(float)
            for r in invs:
                cc = r.get("CompanyCode") or "unknown"
                st = r.get("SupplierInvoiceStatus") or "none"
                by_status[(cc, st)] += 1
                try:
                    amt[(cc, r.get("DocumentCurrency") or r.get("Currency") or "none")] += float(
                        r.get("InvoiceGrossAmount") or 0
                    )
                except (TypeError, ValueError):
                    pass
            self.gauge("sap.s4hana.p2p.supplier_invoices.total", len(invs), tags=tags)
            for (cc, st), c in by_status.items():
                self.gauge(
                    "sap.s4hana.p2p.supplier_invoices.count",
                    c,
                    tags=tags + ["company_code:%s" % cc, "invoice_status:%s" % st],
                )
            for (cc, cur), a in amt.items():
                self.gauge(
                    "sap.s4hana.p2p.supplier_invoice_gross_amount",
                    a,
                    tags=tags + ["company_code:%s" % cc, "currency:%s" % cur],
                )
        except Exception as exc:  # noqa: BLE001
            ok = False
            self.log.error("sap_s4hana_p2p: supplier invoices failed: %s", exc)

        # ---- Bounded per-PO flow logs (PR -> PO -> Invoice) ----
        logs = []
        stuck = 0
        for po in pos[:join_sample]:
            pon = po.get("PurchaseOrder")
            if not pon:
                continue
            try:
                items = fetch("API_PURCHASEORDER_PROCESS_SRV/A_PurchaseOrder('%s')/to_PurchaseOrderItem" % pon, top=1)
            except Exception:
                items = []
            pr = items[0].get("PurchaseRequisition") if items and items[0].get("PurchaseRequisition") else None
            po_date = as_date(po.get("PurchaseOrderDate")) or as_date(po.get("CreationDate"))
            org = po.get("PurchasingOrganization") or "unknown"
            inv = None
            inv_date = None
            try:
                iv = fetch(
                    "API_SUPPLIERINVOICE_PROCESS_SRV/A_SupplierInvoice?$filter=SupplierInvoiceIDByInvcgParty eq '%s'&$top=1"
                    % pon,
                    top=1,
                )
                if iv:
                    inv = iv[0].get("SupplierInvoice")
                    inv_date = as_date(iv[0].get("PostingDate"))
            except Exception:
                pass
            steps = [
                {
                    "category": "PR",
                    "label": "Purchase Requisition",
                    "doc": pr,
                    "state": "complete" if pr else "pending",
                },
                {"category": "PO", "label": "Purchase Order", "doc": pon, "state": "complete", "at": iso(po_date)},
                {
                    "category": "IR",
                    "label": "Supplier Invoice",
                    "doc": inv,
                    "state": "complete" if inv else "pending",
                    "at": iso(inv_date),
                },
            ]
            flow_status = "complete" if inv else "in_progress"
            if flow_status != "complete":
                stuck += 1
            logs.append(
                {
                    "ddsource": "sap_s4hana",
                    "service": "p2p",
                    "sap_sid": sid,
                    "purchasing_org": org,
                    "p2p_purchase_order": pon,
                    "supplier": po.get("Supplier"),
                    "created": iso(po_date),
                    "flow_status": flow_status,
                    "steps": steps,
                }
            )

        self.gauge("sap.s4hana.p2p.pos_traced", len(logs), tags=tags)
        self.gauge("sap.s4hana.p2p.pos_awaiting_invoice", stuck, tags=tags)
        if logs:
            try:
                d = os.path.dirname(log_file)
                if d and not os.path.isdir(d):
                    os.makedirs(d)
                with open(log_file, "a") as fh:
                    for rec in logs:
                        fh.write(json.dumps(rec) + "\n")
            except Exception as exc:  # noqa: BLE001
                self.log.warning("sap_s4hana_p2p: could not write log file %s: %s", log_file, exc)

        self.service_check(
            "sap.s4hana.p2p.can_connect",
            AgentCheck.OK if ok else AgentCheck.CRITICAL,
            tags=tags + ["base_url:%s" % base],
        )
