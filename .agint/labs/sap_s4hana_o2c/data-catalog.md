# SAP ERP Observability — Data Catalog

Inventory of all data collected so far for the ERP Observability product (starting with
SAP S/4HANA). Validated on a SAP CAL S/4HANA 2025 appliance (client 100, demo data).
Last updated: 2026-07-16.

---

## A. SAP S/4HANA — Order-to-Cash (direct A2X OData) — PRIMARY

Source APIs (read-only): `API_SALES_ORDER_SRV/A_SalesOrder`,
`API_OUTBOUND_DELIVERY_SRV/A_OutbDeliveryHeader`, `API_BILLING_DOCUMENT_SRV/A_BillingDocument`,
item-level flow `A_SalesOrderItem/to_SubsequentProcFlowDocItem`.
Volume seen: 5,777 sales orders · 6,323 deliveries · 5,742 billing docs.
Common tags on every metric: `sap_sid`, `sap_client`, `lab`.

### Metrics (`sap.s4hana.o2c.*`)
| Metric | Type | Tags | Source |
|---|---|---|---|
| `sales_orders.total` | gauge | — | count of A_SalesOrder |
| `sales_orders.count` | gauge | sales_org, sd_status, order_type | OverallSDProcessStatus / SalesOrderType |
| `open_sales_documents` | gauge | sales_org | OverallSDProcessStatus≠C |
| `orders_rejected` | gauge | sales_org | OverallSDDocumentRejectionSts∈{B,C} |
| `sales_order_net_amount` | gauge | sales_org, currency | TotalNetAmount |
| `deliveries.total` | gauge | — | A_OutbDeliveryHeader |
| `unbilled_deliveries` | gauge | — | GoodsMovement=C & billing-rel≠C |
| `deliveries_overdue_pgi` | gauge | — | PlannedGoodsIssueDate<today & GM≠C |
| `deliveries_incomplete` | gauge | — | any Hdr*IncompletionStatus≠C |
| `deliveries_not_picked` | gauge | — | OverallPickingStatus≠C |
| `billing_docs.total` | gauge | — | A_BillingDocument |
| `billing_not_posted` | gauge | company_code | AccountingPostingStatus≠C |
| `billing_transfer_failed` | gauge | — | AccountingTransferStatus∉{C,∅} |
| `billing_cancelled` | gauge | — | BillingDocumentIsCancelled |
| `billed_net_amount` | gauge | company_code, currency | TotalNetAmount |
| `orders_traced` | gauge | — | flow join sample size |
| `orders_stuck` | gauge | — | flow_status≠complete |
| `lead_time.order_to_delivery` | histogram (avg/median/95p/max/count) | sales_org | flow dates |
| `lead_time.delivery_to_invoice` | histogram | sales_org | flow dates |
| `lead_time.order_to_invoice` | histogram | sales_org | flow dates |
| `can_connect` | service_check | — | crawler health |

### Logs — per-order document flow (`source:sap_s4hana service:o2c`)
Per-record fields:
- `o2c_sales_order`, `sales_org`, `created`, `amount`, `currency`, `sap_sid`
- `flow_status` (complete/in_progress), `current_step`
- `order_summary[]` — order-level roll-up: `{category, label, state (complete/partial/pending), items_complete, items_total}`
- `items[]` — per line item: `{item, material, flow_status, steps[]}`; each step = `{seq, category, label, doc, state, at}`
- Steps are DYNAMIC (from SAP's document flow); step type = standard doc category
  (C=Order, J=Delivery, R=Goods Movement, M=Invoice, O/K=Credit Memo, F=Contract, H=Returns, …).

### Raw fields available per entity (collectable superset — not all wired)
- Sales Order: SalesOrder, SalesOrderType, SalesOrganization(VKORG), DistributionChannel,
  OrganizationDivision, SoldToParty, PurchaseOrderByCustomer, CreationDate, TotalNetAmount,
  TransactionCurrency, OverallSDProcessStatus, OverallDeliveryStatus, OverallTotalDeliveryStatus,
  OverallSDDocumentRejectionSts. Item: Material, Plant(WERKS), quantities, item category, net amounts.
- Delivery: DeliveryDocument, DeliveryDocumentType, DocumentDate, PlannedGoodsIssueDate, PickingDate,
  LoadingDate, TransportationPlanningDate, SoldToParty, OverallGoodsMovementStatus,
  OverallPickingStatus, OverallDelivReltdBillgStatus, OverallSDProcessStatus, 6× Hdr*IncompletionStatus
  (+ item incompletion), CreditControlArea.
- Billing: BillingDocument, BillingDocumentType, BillingDocumentCategory, BillingDocumentDate,
  CreationDate, SalesOrganization, CompanyCode(BUKRS), TotalNetAmount, TaxAmount, TotalGrossAmount,
  TransactionCurrency, PayerParty, PaymentReference, AccountingPostingStatus, AccountingTransferStatus,
  OverallBillingStatus, BillingDocumentIsCancelled, DocumentReferenceID.
- Document-flow node: SubsequentDocument(+Item), SubsequentDocumentCategory, ProcessFlowLevel,
  RelatedProcFlowDocStsFieldName, SDProcessStatus, AccountingTransferStatus, PrelimBillingDocumentStatus,
  predecessor doc/item/category, CreationDate/Time.

### Dimensions (tagging vocabulary)
sales_org (VKORG) · company_code (BUKRS) · plant (WERKS, item-level, not yet tagged) ·
distribution_channel · division · order_type · billing_type · material · sold_to / payer ·
currency · document_category · credit_control_area · o2c_sales_order (correlation id).

---

## B. SAP S/4HANA — technical / Basis (SAPControl SOAP)
Orthogonal to O2C (S/4 health is ALM's domain per the brief), but collected + real.
Tags: `sap_sid`, `sap_instance`.
- `sap.sapcontrol.process.status` / `.count`, `sap.sapcontrol.instance.status` (process/instance up-down health across D00/ASCS01/HDB02 incl. HANA processes)
- `sap.abap.workprocess.count` (tags type, status)
- `sap.abap.queue.{now,high,max,reads,writes}` (tag queue)
- `sap.abap.enqueue.{locks_now/high/max, enqueue_requests, enqueue_rejects, dequeue_requests, dequeue_all_requests, backup_requests, reporting_requests, lock_time, lock_wait_time, server_time, *_state}`
- `sap.abap.alert.status` (CCMS alert tree, ~313 nodes; tag alert) + `sap.abap.alert.value` (numeric nodes; tag unit)
- `sap.sapcontrol.can_connect` (service check)

---

## C. SAP Cloud ALM (public sandbox — demo data)
Blocked from real breadth until a tenant exists. Tags: `provider`.
- `sap.alm.counter` (EXM_DATAPROVIDER exception counts; tag use_case)
- `sap.alm.count` (DEMO_TASKS; tags status, project)
- `sap.alm.can_connect` (service check)

---

## Available but NOT yet collected (S/4HANA)
- Cash/GL posting — Journal Entry API (`API_JOURNALENTRYITEMBASIC_SRV`): true accounting/cash step + PostingDate + BUKRS.
- Plant (WERKS) + material + item quantities — item-level; validated, not yet tagged/emitted.
- Operational runtime (brief's 2nd bucket): background jobs (runs/delays/failures), async interfaces/IDocs (processed vs failed).
- Document blocks: delivery block, billing block, credit block — fields exist on the docs, not surfaced.

## Caveats
- Appliance demo data is static/historical (2017–18), day-granular, with unrealistic date
  sequencing (billing before PGI) → lead-time VALUES are noisy; schema/wiring correct. Real
  customer S/4HANA yields meaningful values with identical collection code.
- Document flow shows only steps that HAVE happened; greyed "future" stepper circles would need a
  per-doc-type expected-sequence template (copy-control config, not in these APIs).
- Lab auth uses `SAP*`; production needs a dedicated read-only OData user.
