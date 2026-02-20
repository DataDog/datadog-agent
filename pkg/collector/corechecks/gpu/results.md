Loaded metrics spec: 76 entries
Loaded architecture spec: 8 architectures
Querying Datadog API at datadoghq.com...
Generated combinations: 20 (18 known, 2 discovered)

-- ada/physical --
    Scalar query batches: 1/1...
  devices: 232
  expected metrics: 74
  present metrics: 51
  missing metrics: 23
  unknown metrics: 4
  tag failures: 6
  missing metric names:
    - MISSING gpu.decoder_active
    - MISSING gpu.encoder_active
    - MISSING gpu.fan_speed
    - MISSING gpu.fp16_active
    - MISSING gpu.fp32_active
    - MISSING gpu.fp64_active
    - MISSING gpu.integer_active
    - MISSING gpu.memory.temperature
    - MISSING gpu.nvlink.errors.crc.data
    - MISSING gpu.nvlink.errors.crc.flit
    - MISSING gpu.nvlink.errors.ecc
    - MISSING gpu.nvlink.errors.recovery
    - MISSING gpu.nvlink.errors.replay
    - MISSING gpu.nvlink.speed
    - MISSING gpu.nvlink.throughput.data.rx
    - MISSING gpu.nvlink.throughput.data.tx
    - MISSING gpu.nvlink.throughput.raw.rx
    - MISSING gpu.nvlink.throughput.raw.tx
    - MISSING gpu.process.decoder_active
    - MISSING gpu.process.encoder_active
    - MISSING gpu.sm_occupancy
    - MISSING gpu.sm_utilization
    - MISSING gpu.tensor_active
  unknown metric names:
    - UNKNOWN gpu.decoder_utilization
    - UNKNOWN gpu.encoder_utilization
    - UNKNOWN gpu.process.decoder_utilization
    - UNKNOWN gpu.process.encoder_utilization
  tag failure details:
    - TAG FAIL gpu.core.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.memory.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.core.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.dram_active: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.memory.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.sm_active: missing/non-null [container_id, container_name]

-- ampere/mig --
  devices: 0
  expected metrics: 17
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- ampere/physical --
    Scalar query batches: 1/1...
  devices: 306
  expected metrics: 67
  present metrics: 63
  missing metrics: 4
  unknown metrics: 4
  tag failures: 4
  missing metric names:
    - MISSING gpu.decoder_active
    - MISSING gpu.encoder_active
    - MISSING gpu.process.decoder_active
    - MISSING gpu.process.encoder_active
  unknown metric names:
    - UNKNOWN gpu.decoder_utilization
    - UNKNOWN gpu.encoder_utilization
    - UNKNOWN gpu.process.decoder_utilization
    - UNKNOWN gpu.process.encoder_utilization
  tag failure details:
    - TAG FAIL gpu.process.core.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.dram_active: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.memory.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.sm_active: missing/non-null [container_id, container_name]

-- ampere/vgpu --
    Scalar query batches: 1/1...
  devices: 34
  expected metrics: 61
  present metrics: 24
  missing metrics: 37
  unknown metrics: 6
  tag failures: 5
  missing metric names:
    - MISSING gpu.clock.speed.graphics.max
    - MISSING gpu.clock.speed.memory.max
    - MISSING gpu.clock.speed.sm.max
    - MISSING gpu.clock.speed.video.max
    - MISSING gpu.clock.throttle_reasons.applications_clocks_setting
    - MISSING gpu.clock.throttle_reasons.display_clock_setting
    - MISSING gpu.clock.throttle_reasons.gpu_idle
    - MISSING gpu.clock.throttle_reasons.none
    - MISSING gpu.clock.throttle_reasons.sw_power_cap
    - MISSING gpu.clock.throttle_reasons.sw_thermal_slowdown
    - MISSING gpu.clock.throttle_reasons.sync_boost
    - MISSING gpu.errors.xid.total
    - MISSING gpu.fan_speed
    - MISSING gpu.memory.temperature
    - MISSING gpu.nvlink.errors.crc.data
    - MISSING gpu.nvlink.errors.crc.flit
    - MISSING gpu.nvlink.errors.ecc
    - MISSING gpu.nvlink.errors.recovery
    - MISSING gpu.nvlink.errors.replay
    - MISSING gpu.nvlink.nvswitch_connected
    - MISSING gpu.nvlink.speed
    - MISSING gpu.nvlink.throughput.data.rx
    - MISSING gpu.nvlink.throughput.data.tx
    - MISSING gpu.nvlink.throughput.raw.rx
    - MISSING gpu.nvlink.throughput.raw.tx
    - MISSING gpu.pci.replay_counter
    - MISSING gpu.pci.throughput.rx
    - MISSING gpu.pci.throughput.tx
    - MISSING gpu.power.management_limit
    - MISSING gpu.power.usage
    - MISSING gpu.remapped_rows.correctable
    - MISSING gpu.remapped_rows.failed
    - MISSING gpu.remapped_rows.pending
    - MISSING gpu.remapped_rows.uncorrectable
    - MISSING gpu.slowdown_temperature
    - MISSING gpu.temperature
    - MISSING gpu.total_energy_consumption
  unknown metric names:
    - UNKNOWN gpu.decoder_utilization
    - UNKNOWN gpu.dram_active
    - UNKNOWN gpu.encoder_utilization
    - UNKNOWN gpu.process.decoder_utilization
    - UNKNOWN gpu.process.dram_active
    - UNKNOWN gpu.process.encoder_utilization
  tag failure details:
    - TAG FAIL gpu.core.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.memory.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.core.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.memory.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.sm_active: missing/non-null [container_id, container_name]

-- blackwell/physical --
    Scalar query batches: 1/1...
  devices: 112
  expected metrics: 74
  present metrics: 62
  missing metrics: 12
  unknown metrics: 4
  tag failures: 6
  missing metric names:
    - MISSING gpu.decoder_active
    - MISSING gpu.encoder_active
    - MISSING gpu.errors.xid.total
    - MISSING gpu.fan_speed
    - MISSING gpu.nvlink.errors.crc.data
    - MISSING gpu.nvlink.errors.crc.flit
    - MISSING gpu.nvlink.errors.ecc
    - MISSING gpu.nvlink.errors.recovery
    - MISSING gpu.nvlink.errors.replay
    - MISSING gpu.nvlink.speed
    - MISSING gpu.process.decoder_active
    - MISSING gpu.process.encoder_active
  unknown metric names:
    - UNKNOWN gpu.decoder_utilization
    - UNKNOWN gpu.encoder_utilization
    - UNKNOWN gpu.process.decoder_utilization
    - UNKNOWN gpu.process.encoder_utilization
  tag failure details:
    - TAG FAIL gpu.core.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.memory.limit: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.core.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.dram_active: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.memory.usage: missing/non-null [container_id, container_name]
    - TAG FAIL gpu.process.sm_active: missing/non-null [container_id, container_name]

-- fermi/physical --
  devices: 0
  expected metrics: 22
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- fermi/vgpu --
  devices: 0
  expected metrics: 22
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- hopper/mig --
  devices: 0
  expected metrics: 24
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- hopper/physical --
  devices: 0
  expected metrics: 74
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- hopper/vgpu --
  devices: 0
  expected metrics: 68
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- kepler/physical --
  devices: 0
  expected metrics: 45
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- kepler/vgpu --
  devices: 0
  expected metrics: 39
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- maxwell/physical --
  devices: 0
  expected metrics: 45
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- maxwell/vgpu --
  devices: 0
  expected metrics: 39
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- pascal/physical --
  devices: 0
  expected metrics: 58
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- pascal/vgpu --
  devices: 0
  expected metrics: 52
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- turing/physical --
  devices: 0
  expected metrics: 60
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- turing/vgpu --
  devices: 0
  expected metrics: 54
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- volta/physical --
  devices: 0
  expected metrics: 60
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

-- volta/vgpu --
  devices: 0
  expected metrics: 54
  present metrics: skipped (no devices)
  missing metrics: skipped (no devices)

Summary:

| architecture   | device mode   | status   |   found devices | missing/known/unknown metrics   |   tag failures |
|----------------|---------------|----------|-----------------|---------------------------------|----------------|
| ada            | physical      | [33munknown[0m  |             232 | [31m23[0m/[32m74[0m/[33m4[0m                         |              [31m6[0m |
| ampere         | mig           | [33mmissing[0m  |               0 | 0/[32m17[0m/0                          |              0 |
| ampere         | physical      | [31mfail[0m     |             306 | [31m4[0m/[32m67[0m/[33m4[0m                          |              [31m4[0m |
| ampere         | vgpu          | [31mfail[0m     |              34 | [31m37[0m/[32m61[0m/[33m6[0m                         |              [31m5[0m |
| blackwell      | physical      | [33munknown[0m  |             112 | [31m12[0m/[32m74[0m/[33m4[0m                         |              [31m6[0m |
| fermi          | physical      | [33mmissing[0m  |               0 | 0/[32m22[0m/0                          |              0 |
| fermi          | vgpu          | [33mmissing[0m  |               0 | 0/[32m22[0m/0                          |              0 |
| hopper         | mig           | [33mmissing[0m  |               0 | 0/[32m24[0m/0                          |              0 |
| hopper         | physical      | [33mmissing[0m  |               0 | 0/[32m74[0m/0                          |              0 |
| hopper         | vgpu          | [33mmissing[0m  |               0 | 0/[32m68[0m/0                          |              0 |
| kepler         | physical      | [33mmissing[0m  |               0 | 0/[32m45[0m/0                          |              0 |
| kepler         | vgpu          | [33mmissing[0m  |               0 | 0/[32m39[0m/0                          |              0 |
| maxwell        | physical      | [33mmissing[0m  |               0 | 0/[32m45[0m/0                          |              0 |
| maxwell        | vgpu          | [33mmissing[0m  |               0 | 0/[32m39[0m/0                          |              0 |
| pascal         | physical      | [33mmissing[0m  |               0 | 0/[32m58[0m/0                          |              0 |
| pascal         | vgpu          | [33mmissing[0m  |               0 | 0/[32m52[0m/0                          |              0 |
| turing         | physical      | [33mmissing[0m  |               0 | 0/[32m60[0m/0                          |              0 |
| turing         | vgpu          | [33mmissing[0m  |               0 | 0/[32m54[0m/0                          |              0 |
| volta          | physical      | [33mmissing[0m  |               0 | 0/[32m60[0m/0                          |              0 |
| volta          | vgpu          | [33mmissing[0m  |               0 | 0/[32m54[0m/0                          |              0 |

Combinations with metric/tag failures (and devices present): 2
2026/02/20 17:27:53 could not run command: exit status 1
