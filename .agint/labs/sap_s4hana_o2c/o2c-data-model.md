# SAP S/4HANA Order-to-Cash (O2C) — Datadog data model (frontend design input)

Source: direct S/4HANA **A2X OData** APIs (read-only), NOT Cloud ALM, NOT SAPControl.
Validated against the CAL S/4HANA 2025 appliance (client 100, ~5,777 sales orders,
best-practices demo data 2017–2018). Sales Order API live; Delivery/Billing pending
service activation (see runbook).

## 1. The O2C stages → API source → key fields

| Stage | A2X OData service / entity | Key ID | Timestamp(s) | Status field(s) | Amount | Org fields |
|-------|----------------------------|--------|--------------|-----------------|--------|-----------|
| **Order** | `API_SALES_ORDER_SRV` / `A_SalesOrder` (+ `A_SalesOrderItem` for plant) | `SalesOrder` | `CreationDate`(+`CreationTime`) | `OverallSDProcessStatus`, `OverallDeliveryStatus`, `OverallSDDocumentRejectionSts` | `TotalNetAmount` + `TransactionCurrency` | `SalesOrganization` (VKORG), `DistributionChannel`, `OrganizationDivision`; `Plant` (WERKS) at item |
| **Delivery / PGI** | `API_OUTBOUND_DELIVERY_SRV` / `A_OutbDeliveryHeader` | `OutboundDelivery` | `ActualGoodsMovementDate` (PGI), `DeliveryDate`, `CreationDate` | `OverallGoodsMovementStatus`, `OverallSDProcessStatus` | (item level) | `Plant` (item), refs sales order |
| **Billing** | `API_BILLING_DOCUMENT_SRV` / `A_BillingDocument` | `BillingDocument` | `BillingDocumentDate` | `OverallSDProcessStatus`, `AccountingPostingStatus` | `TotalNetAmount` | `SalesOrganization`, `CompanyCode` (BUKRS) |
| **Cash / accounting** | `API_JOURNALENTRYITEMBASIC_SRV` (or `API_OPLACCTGDOCITEMCUBE_SRV`) | `AccountingDocument` | `PostingDate` | posted | amount | `CompanyCode` (BUKRS) |

SAP quirks: dates are `/Date(epoch_ms)/` (decode to ISO); many are **day-granular**
(no time) → lead times are effectively in **days**, not sub-second. `CreationTime` was
null on sampled orders.

## 2. Correlation (the "full path") — VALIDATED on the appliance

Confirmed working links (use the item-level **navigation properties**, NOT the standalone
`A_*Item` entity sets, which didn't filter):
- SO ← Delivery: `A_OutbDeliveryHeader('<dlv>')/to_DeliveryDocumentItem` → item
  `ReferenceSDDocument` = SalesOrder (`ReferenceSDDocumentCategory=C`).
- Delivery ← Billing: `A_BillingDocument('<bill>')/to_Item` → item `ReferenceSDDocument`
  = Delivery number (F2 delivery-related billing).
- So the chain is stitched **billing → delivery → sales order**; join key = the delivery
  number, and the sales order threads through as the top correlation id.
- Fetch a sales order by key: `A_SalesOrder('<n>')` (the `$filter=SalesOrder eq '<n>'`
  form returned empty — use the key form).

Real validated example (appliance, client 100):
`SO 22 (9170 USD, VKORG 1710, created 2017-10-08) → Dlv 80000000 (PGI 2017-10-10) →
Bill 90000000 (2017-10-08, AccountingPostingStatus=C)`. 8/8 sampled orders linked.

⚠️ **Demo-data caveat:** dates are day-granular and NOT realistically sequenced (billing
dated before PGI → negative PGI→Bill lead time in the demo). The model/math are correct;
lead-time *values* are only meaningful on real customer data. Design against the model.

## 2b. Correlation (conceptual)

Chain by document reference (the SAP "document flow"):
`SalesOrder → OutboundDelivery (ref sales order) → BillingDocument (ref delivery/order) → AccountingDocument (ref billing)`.
- Join key that threads through everything: **`SalesOrder`** (carried as a reference on
  successor docs, at item level via `ReferenceSDDocument`).
- Correlation tag to stamp on every emitted signal: `o2c_sales_order:<SalesOrder>`.

## 3. Datadog modeling — two layers

### (a) Aggregates → metrics (gauges + distributions, per poll)
Counts / aging (the brief's O2C-pressure KPIs):
- `sap.s4.o2c.sales_orders.count` — tags: `status`, `sales_org`, `order_type`
- `sap.s4.o2c.open_sales_documents` — OverallSDProcessStatus != C
- `sap.s4.o2c.deliveries_overdue_pgi`
- `sap.s4.o2c.unbilled_deliveries`
- `sap.s4.o2c.billing_not_posted` (AccountingPostingStatus != posted)
- `sap.s4.o2c.order_net_amount` (sum) — tags: `sales_org`, `currency`
Lead times (distribution metrics → p50/p95/p99 in-platform):
- `sap.s4.o2c.lead_time.so_to_pgi` (days)
- `sap.s4.o2c.lead_time.pgi_to_billing` (days)
- `sap.s4.o2c.lead_time.so_to_cash` (days)
- `sap.s4.o2c.order_age` (days, open orders)

### (b) Per-order path → structured events/logs (+ optional trace)
Each document at each stage → one structured **log/event**, all sharing
`o2c_sales_order:<n>`, so the frontend reconstructs the **orders table** and the
**per-order timeline** by faceting:
- fields: stage, doc id, timestamp, status, amount, currency, sold_to, all org tags.
- Powers: orders table (Flow status, Age, Amount, Delivery/Billing status), and the
  step-by-step SO→PGI→Billing timeline with IDs+timestamps.
- **Optional APM trace per order** for a waterfall view (span per stage). Caveat:
  day-granular timestamps + multi-day/long-lived orders make traces unconventional;
  events/logs are the pragmatic core, trace is a nice-to-have for the timeline widget.

## 4. Consistent tag model (align with the ALM brief)
`tenant`, `system_id` / `sap_sid`, `sales_org` (VKORG), `plant` (WERKS),
`company_code` (BUKRS), `distribution_channel`, `order_type`, `sold_to`,
`o2c_sales_order`. Same tag vocabulary as the ALM Health/BPMon integration so the two
correlate.

## 5. Requirements → assets (from the S/4HANA brief)
- O2C KPIs (Open Sales Docs, Deliveries Overdue PGI, Unbilled Deliveries, Billing Not
  Posted) → the 4 gauge metrics, tagged plant/sales_org/company_code.
- Slice by WERKS/VKORG/BUKRS → tags above.
- p50/p95/p99 SO→PGI, PGI→Billing → the lead_time distribution metrics.
- Orders table (status/age/amount/delivery+billing status) → events/logs facets.
- Anomaly/threshold alerts on Open Sales Docs + Unbilled Deliveries by plant/sales_org
  → monitors on those gauges.
- Roll-ups (top plants/sales orgs by backlog) + burn-down → dashboard widgets over the
  gauges + events.

## 6. Realities for the demo
- Appliance demo data is **static/historical** (2017–2018) — no live movement. Good for
  frontend *design*; a live/trending demo needs order generation (load).
- Sales Order OData works today (SAP* / client 100). Delivery + Billing return 403 until
  their Gateway services are activated (see the activation runbook).
- Auth for a real integration should be a **dedicated read-only OData user** (not SAP*),
  with authorization for the specific `/IWFND` services + business-object read.
