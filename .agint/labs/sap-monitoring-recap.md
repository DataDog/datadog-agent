# SAP Monitoring — Build Recap (2026-07-15)

Goal: Datadog SAP monitoring — **Cloud ALM** (health/BPMon KPIs, needs tenant) + **S/4HANA Order-to-Cash** via direct A2X OData (no tenant). O2C is the main focus.

## Built (live)
- **S/4HANA O2C crawler** (`sap_s4hana_o2c`) ⭐ — polls A2X OData (Sales Order / Delivery / Billing). Emits:
  - KPI metrics `sap.s4hana.o2c.*` (open docs, rejected, unbilled, overdue PGI, not-posted, amounts, orders stuck…)
  - Lead-time histograms (order→delivery→invoice, p50/p95)
  - **Dynamic per-order document-flow logs** (`source:sap_s4hana service:o2c`) — steps built from SAP's own doc flow (config-agnostic), with order-level roll-up + per-item steps + `flow_status`/`current_step`. Backs the stepper UI.
  - Files: `.agint/labs/sap_s4hana_o2c/` (+ `o2c-data-model.md`).
- **Dashboard**: https://app.datadoghq.com/dashboard/8ij-qac-uzb — staged pipeline (Overview → Order → Delivery → Billing → Cash → Lead times → Flow logs). No monitors.
- **SAPControl check** (`sap_abap`) — ABAP/Basis metrics (work processes, queues, enqueue, CCMS, HANA process health). Real but orthogonal to the product (S/4 health is ALM's job).
- **Cloud ALM check** (`sap_alm`) — works against the public sandbox (demo data); OAuth2-ready. Real breadth blocked on a tenant.
- **CAL IAM**: cloud-inventory PR #64132 merged (IAM user for the appliance).

## Infra
- **S/4HANA 2025 CAL appliance** live (SID S4H, r6i.8xlarge, dynamic IP, `SAP*` creds, agent → datadoghq.com). Demo data 2017–18, client 100.

## Not done / parked
- Real Cloud ALM tenant (blocked — colleague's license = likely unblock)
- HANA DB metrics (`sap-hana` hdbcli check)
- Operational-runtime half (jobs/interfaces/IDocs)
- Monitors
- Prod hardening: read-only OData user (not SAP*), EIP, logrotate/emit-on-change, integration-repo home. Nothing committed — all lab artifacts.

## Caveats
- Appliance demo data is static/day-granular with unrealistic date sequencing → lead-time values noisy (wiring correct; real data fixes it).
- Doc flow shows only steps that happened; greyed "future" stepper circles need a per-doc-type template (not in these APIs).
