use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use arrow::array::{ArrayRef, BooleanArray, Int32Array, Int64Array, UInt32Array, UInt64Array};
use arrow::datatypes::{DataType, Field, Schema};
use arrow::record_batch::RecordBatch;

use super::intern::StringInterner;
use super::parquet_helpers::{dict_utf8_type, interner_to_dict_array, BaseWriter};
use super::thread::{SignalWriter, WriterStats};
use crate::generated::signals_generated::signals;

/// Schema for the connections Parquet file (27 columns).
fn connections_schema() -> Arc<Schema> {
    let dt = dict_utf8_type();
    Arc::new(Schema::new(vec![
        Field::new("pid", DataType::Int32, false),
        Field::new("local_ip", dt.clone(), false),
        Field::new("local_port", DataType::Int32, false),
        Field::new("local_container_id", dt.clone(), false),
        Field::new("remote_ip", dt.clone(), false),
        Field::new("remote_port", DataType::Int32, false),
        Field::new("remote_container_id", dt.clone(), false),
        Field::new("family", DataType::UInt32, false),
        Field::new("conn_type", DataType::UInt32, false),
        Field::new("direction", DataType::UInt32, false),
        Field::new("net_ns", DataType::UInt32, false),
        Field::new("bytes_sent", DataType::UInt64, false),
        Field::new("bytes_received", DataType::UInt64, false),
        Field::new("packets_sent", DataType::UInt64, false),
        Field::new("packets_received", DataType::UInt64, false),
        Field::new("retransmits", DataType::UInt32, false),
        Field::new("rtt", DataType::UInt32, false),
        Field::new("rtt_var", DataType::UInt32, false),
        Field::new("intra_host", DataType::Boolean, false),
        Field::new("dns_successful_responses", DataType::UInt32, false),
        Field::new("dns_failed_responses", DataType::UInt32, false),
        Field::new("dns_timeouts", DataType::UInt32, false),
        Field::new("dns_success_latency_sum", DataType::UInt64, false),
        Field::new("dns_failure_latency_sum", DataType::UInt64, false),
        Field::new("tcp_established", DataType::UInt32, false),
        Field::new("tcp_closed", DataType::UInt32, false),
        Field::new("timestamp_ns", DataType::Int64, false),
    ]))
}

/// Columnar accumulator for network connection entries.
pub struct ConnectionsWriter {
    pub base: BaseWriter,

    local_ips: StringInterner,
    local_container_ids: StringInterner,
    remote_ips: StringInterner,
    remote_container_ids: StringInterner,

    pids: Vec<i32>,
    local_ports: Vec<i32>,
    remote_ports: Vec<i32>,
    families: Vec<u32>,
    conn_types: Vec<u32>,
    directions: Vec<u32>,
    net_ns: Vec<u32>,
    bytes_sent: Vec<u64>,
    bytes_received: Vec<u64>,
    packets_sent: Vec<u64>,
    packets_received: Vec<u64>,
    retransmits: Vec<u32>,
    rtts: Vec<u32>,
    rtt_vars: Vec<u32>,
    intra_hosts: Vec<bool>,
    dns_successful_responses: Vec<u32>,
    dns_failed_responses: Vec<u32>,
    dns_timeouts: Vec<u32>,
    dns_success_latency_sums: Vec<u64>,
    dns_failure_latency_sums: Vec<u64>,
    tcp_established: Vec<u32>,
    tcp_closed: Vec<u32>,
    timestamps: Vec<i64>,
}

impl ConnectionsWriter {
    pub fn new(
        output_dir: impl AsRef<Path>,
        flush_rows: usize,
        flush_interval: Duration,
    ) -> Self {
        let output_dir = output_dir.as_ref();
        Self {
            base: BaseWriter::new(output_dir, flush_rows, flush_interval),

            local_ips: StringInterner::with_capacity(flush_rows),
            local_container_ids: StringInterner::with_capacity(flush_rows),
            remote_ips: StringInterner::with_capacity(flush_rows),
            remote_container_ids: StringInterner::with_capacity(flush_rows),

            pids: Vec::with_capacity(flush_rows),
            local_ports: Vec::with_capacity(flush_rows),
            remote_ports: Vec::with_capacity(flush_rows),
            families: Vec::with_capacity(flush_rows),
            conn_types: Vec::with_capacity(flush_rows),
            directions: Vec::with_capacity(flush_rows),
            net_ns: Vec::with_capacity(flush_rows),
            bytes_sent: Vec::with_capacity(flush_rows),
            bytes_received: Vec::with_capacity(flush_rows),
            packets_sent: Vec::with_capacity(flush_rows),
            packets_received: Vec::with_capacity(flush_rows),
            retransmits: Vec::with_capacity(flush_rows),
            rtts: Vec::with_capacity(flush_rows),
            rtt_vars: Vec::with_capacity(flush_rows),
            intra_hosts: Vec::with_capacity(flush_rows),
            dns_successful_responses: Vec::with_capacity(flush_rows),
            dns_failed_responses: Vec::with_capacity(flush_rows),
            dns_timeouts: Vec::with_capacity(flush_rows),
            dns_success_latency_sums: Vec::with_capacity(flush_rows),
            dns_failure_latency_sums: Vec::with_capacity(flush_rows),
            tcp_established: Vec::with_capacity(flush_rows),
            tcp_closed: Vec::with_capacity(flush_rows),
            timestamps: Vec::with_capacity(flush_rows),
        }
    }

    #[inline]
    pub fn len(&self) -> usize {
        self.timestamps.len()
    }

    pub fn push(&mut self, batch: &signals::ConnectionBatch<'_>) -> Result<()> {
        if let Some(entries) = batch.entries() {
            for i in 0..entries.len() {
                let e = entries.get(i);

                self.local_ips.intern(e.local_ip().unwrap_or(""));
                self.local_container_ids
                    .intern(e.local_container_id().unwrap_or(""));
                self.remote_ips.intern(e.remote_ip().unwrap_or(""));
                self.remote_container_ids
                    .intern(e.remote_container_id().unwrap_or(""));

                self.pids.push(e.pid());
                self.local_ports.push(e.local_port());
                self.remote_ports.push(e.remote_port());
                self.families.push(e.family());
                self.conn_types.push(e.conn_type());
                self.directions.push(e.direction());
                self.net_ns.push(e.net_ns());
                self.bytes_sent.push(e.bytes_sent());
                self.bytes_received.push(e.bytes_received());
                self.packets_sent.push(e.packets_sent());
                self.packets_received.push(e.packets_received());
                self.retransmits.push(e.retransmits());
                self.rtts.push(e.rtt());
                self.rtt_vars.push(e.rtt_var());
                self.intra_hosts.push(e.intra_host());
                self.dns_successful_responses
                    .push(e.dns_successful_responses());
                self.dns_failed_responses.push(e.dns_failed_responses());
                self.dns_timeouts.push(e.dns_timeouts());
                self.dns_success_latency_sums
                    .push(e.dns_success_latency_sum());
                self.dns_failure_latency_sums
                    .push(e.dns_failure_latency_sum());
                self.tcp_established.push(e.tcp_established());
                self.tcp_closed.push(e.tcp_closed());
                self.timestamps.push(e.timestamp_ns());
            }
        }

        if self.base.should_flush(self.len()) {
            self.flush()?;
        }
        Ok(())
    }

    pub fn flush(&mut self) -> Result<()> {
        let row_count = self.len();
        if row_count == 0 {
            anyhow::bail!("no rows to flush");
        }

        let (local_ip_vals, local_ip_codes) = self.local_ips.take();
        let (local_cid_vals, local_cid_codes) = self.local_container_ids.take();
        let (remote_ip_vals, remote_ip_codes) = self.remote_ips.take();
        let (remote_cid_vals, remote_cid_codes) = self.remote_container_ids.take();

        let pids = std::mem::take(&mut self.pids);
        let local_ports = std::mem::take(&mut self.local_ports);
        let remote_ports = std::mem::take(&mut self.remote_ports);
        let families = std::mem::take(&mut self.families);
        let conn_types = std::mem::take(&mut self.conn_types);
        let directions = std::mem::take(&mut self.directions);
        let net_ns = std::mem::take(&mut self.net_ns);
        let bytes_sent = std::mem::take(&mut self.bytes_sent);
        let bytes_received = std::mem::take(&mut self.bytes_received);
        let packets_sent = std::mem::take(&mut self.packets_sent);
        let packets_received = std::mem::take(&mut self.packets_received);
        let retransmits = std::mem::take(&mut self.retransmits);
        let rtts = std::mem::take(&mut self.rtts);
        let rtt_vars = std::mem::take(&mut self.rtt_vars);
        let intra_hosts = std::mem::take(&mut self.intra_hosts);
        let dns_successful_responses = std::mem::take(&mut self.dns_successful_responses);
        let dns_failed_responses = std::mem::take(&mut self.dns_failed_responses);
        let dns_timeouts = std::mem::take(&mut self.dns_timeouts);
        let dns_success_latency_sums = std::mem::take(&mut self.dns_success_latency_sums);
        let dns_failure_latency_sums = std::mem::take(&mut self.dns_failure_latency_sums);
        let tcp_established = std::mem::take(&mut self.tcp_established);
        let tcp_closed = std::mem::take(&mut self.tcp_closed);
        let timestamps = std::mem::take(&mut self.timestamps);

        // Sort by timestamp for better compression.
        let mut order: Vec<usize> = (0..row_count).collect();
        order.sort_unstable_by_key(|&i| timestamps[i]);

        let columns: Vec<ArrayRef> = vec![
            Arc::new(Int32Array::from_iter_values(order.iter().map(|&i| pids[i]))),
            Arc::new(interner_to_dict_array(local_ip_vals, local_ip_codes, &order)),
            Arc::new(Int32Array::from_iter_values(order.iter().map(|&i| local_ports[i]))),
            Arc::new(interner_to_dict_array(local_cid_vals, local_cid_codes, &order)),
            Arc::new(interner_to_dict_array(remote_ip_vals, remote_ip_codes, &order)),
            Arc::new(Int32Array::from_iter_values(order.iter().map(|&i| remote_ports[i]))),
            Arc::new(interner_to_dict_array(remote_cid_vals, remote_cid_codes, &order)),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| families[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| conn_types[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| directions[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| net_ns[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| bytes_sent[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| bytes_received[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| packets_sent[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| packets_received[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| retransmits[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| rtts[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| rtt_vars[i]))),
            Arc::new(BooleanArray::from_iter(order.iter().map(|&i| Some(intra_hosts[i])))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| dns_successful_responses[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| dns_failed_responses[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| dns_timeouts[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| dns_success_latency_sums[i]))),
            Arc::new(UInt64Array::from_iter_values(order.iter().map(|&i| dns_failure_latency_sums[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| tcp_established[i]))),
            Arc::new(UInt32Array::from_iter_values(order.iter().map(|&i| tcp_closed[i]))),
            Arc::new(Int64Array::from_iter_values(order.iter().map(|&i| timestamps[i]))),
        ];

        let schema = connections_schema();
        let batch = RecordBatch::try_new(schema.clone(), columns)
            .context("building connections RecordBatch")?;

        self.base.write_batch("connections", schema, batch)?;
        Ok(())
    }
}

impl SignalWriter for ConnectionsWriter {
    fn process_frame(&mut self, buf: &[u8]) -> Result<()> {
        let env = flatbuffers::root::<signals::SignalEnvelope>(buf)
            .map_err(|e| anyhow::anyhow!("decode error: {e}"))?;
        if let Some(batch) = env.payload_as_connection_batch() {
            self.push(&batch)?;
        }
        Ok(())
    }

    fn flush_and_close(&mut self) -> Result<()> {
        if self.len() > 0 {
            self.flush()?;
        }
        self.base.close()
    }

    fn stats(&self) -> WriterStats {
        WriterStats {
            buffered_rows: self.len() as u64,
            flush_count: self.base.flush_count,
            flush_bytes: self.base.flush_bytes,
            rows_written: self.base.rows_written,
            last_flush_duration_ns: self.base.last_flush_duration_ns,
        }
    }
}
