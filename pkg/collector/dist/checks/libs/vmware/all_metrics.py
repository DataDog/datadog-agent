# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

CPU_METRICS = {
    # CPU Capacity Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.capacity.contention': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # CPU Capacity Demand
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.capacity.demand': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # CPU Capacity entitle vs demand ratio
    # Compatibility: 5.0.0
    'cpu.demandEntitlementRatio': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # CPU Capacity Entitlement
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.capacity.entitlement': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # CPU Capacity Provisioned
    # Compatibility: UNKNOWN
    'cpu.capacity.provisioned': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': []
    },
    # CPU Capacity Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.capacity.usage': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Core Utilization
    # Compatibility: UNKNOWN
    'cpu.coreUtilization': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # CPU Core Count Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.corecount.contention': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # CPU Core Count Provisioned
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.corecount.provisioned': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # CPU Core Count Usage
    # Compatibility: UNKNOWN
    'cpu.corecount.usage': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # Co-stop
    # Compatibility: 5.0.0
    'cpu.costop': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Worst case allocation
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.cpuentitlement': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['ResourcePool']
    },
    # Demand
    # Compatibility: 5.0.0
    'cpu.demand': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Entitlement
    # Compatibility: 5.0.0
    'cpu.entitlement': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Extra
    # Compatibility: 3.5.0
    'cpu.extra': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # Guaranteed
    # Compatibility: 3.5.0
    'cpu.guaranteed': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Idle
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.idle': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Latency
    # Compatibility: 5.0.0
    'cpu.latency': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Max limited
    # Compatibility: 5.0.0
    'cpu.maxlimited': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # Overlap
    # Compatibility: 5.0.0
    'cpu.overlap': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # Ready
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.ready': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Readiness
    # Compatibility: 6.0.0
    'cpu.readiness': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Reserved capacity
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.reservedCapacity': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Run
    # Compatibility: 5.0.0
    'cpu.run': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # Swap wait
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'cpu.swapwait': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # System
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.system': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # Total capacity
    # Compatibility: 4.1.0 / 5.0.0
    'cpu.totalCapacity': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Total
    # Compatibility: UNKNOWN
    'cpu.totalmhz': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': []
    },
    # Usage
    # Compatibility: UNKNOWN
    'cpu.usage': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Usage in MHz
    # Compatibility: UNKNOWN
    'cpu.usagemhz': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Used
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.used': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Utilization
    # Compatibility: UNKNOWN
    'cpu.utilization': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Wait
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'cpu.wait': {
        's_type': 'delta',
        'unit': 'millisecond',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
}


DATASTORE_METRICS = {
    # Bus resets
    # Compatibility: UNKNOWN
    'datastore.busResets': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['Datastore']
    },
    # Datastore Command Aborts
    # Compatibility: UNKNOWN
    'datastore.commandsAborted': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['Datastore']
    },
    # Storage I/O Control aggregated IOPS
    # Compatibility: 4.1.0 / 5.0.0
    'datastore.datastoreIops': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Storage I/O Control datastore maximum queue depth
    # Compatibility: 5.0.0
    'datastore.datastoreMaxQueueDepth': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore normalized read latency
    # Compatibility: 5.0.0
    'datastore.datastoreNormalReadLatency': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore normalized write latency
    # Compatibility: 5.0.0
    'datastore.datastoreNormalWriteLatency': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore bytes read
    # Compatibility: 5.0.0
    'datastore.datastoreReadBytes': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore read I/O rate
    # Compatibility: 5.0.0
    'datastore.datastoreReadIops': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore read workload metric
    # Compatibility: 5.0.0
    'datastore.datastoreReadLoadMetric': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore outstanding read requests
    # Compatibility: 5.0.0
    'datastore.datastoreReadOIO': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore bytes written
    # Compatibility: 5.0.0
    'datastore.datastoreWriteBytes': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore write I/O rate
    # Compatibility: 5.0.0
    'datastore.datastoreWriteIops': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore write workload metric
    # Compatibility: 5.0.0
    'datastore.datastoreWriteLoadMetric': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Storage DRS datastore outstanding write requests
    # Compatibility: 5.0.0
    'datastore.datastoreWriteOIO': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Highest latency
    # Compatibility: 5.0.0
    'datastore.maxTotalLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Average read requests per second
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.numberReadAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Average write requests per second
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.numberWriteAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Read rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.read': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Storage I/O Control normalized latency
    # Compatibility: 4.1.0 / 5.0.0
    'datastore.sizeNormalizedDatastoreLatency': {
        's_type': 'absolute',
        'unit': 'microsecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Datastore Throughput Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.throughput.contention': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['Datastore']
    },
    # Datastore Throughput Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.throughput.usage': {
        's_type': 'absolute',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['Datastore']
    },
    # Read latency
    # Compatibility: 4.1.0 / 5.0.0
    'datastore.totalReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Write latency
    # Compatibility: 4.1.0 / 5.0.0
    'datastore.totalWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Write rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'datastore.write': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Datastore latency observed by VM's
    # Compatibility: 6.0.0
    'datastore.datastoreVMObservedLatency': {
        's_type': 'absolute',
        'unit': 'microsecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Storage I/O Control actively controlled datastore latency
    # Compatibility: 6.0.0
    'datastore.siocActiveTimePercentage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


DISK_METRICS = {
    # Bus resets
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.busResets': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Storage Capacity Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.capacity.contention': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['Datastore']
    },
    # Storage Capacity Provisioned
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.capacity.provisioned': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['Datastore']
    },
    # Storage Capacity Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.capacity.usage': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['Datastore']
    },
    # Commands issued
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.commands': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Commands terminated
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.commandsAborted': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Average commands issued per second
    # Compatibility: 4.1.0 / 5.0.0
    'disk.commandsAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Overhead due to delta disk backings
    # Compatibility: UNKNOWN
    'disk.deltaused': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': []
    },
    # Physical device command latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.deviceLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Physical device read latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.deviceReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Physical device write latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.deviceWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Kernel command latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.kernelLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Kernel read latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.kernelReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Kernel write latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.kernelWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Maximum queue depth
    # Compatibility: 4.1.0 / 5.0.0
    'disk.maxQueueDepth': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Highest latency
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'disk.maxTotalLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Read requests
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.numberRead': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Average read requests per second
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.numberReadAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Write requests
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.numberWrite': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Average write requests per second
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.numberWriteAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Queue command latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.queueLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Queue read latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.queueReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Queue write latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.queueWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Read rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.read': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    # Disk SCSI Reservation Conflicts %
    # Compatibility: UNKNOWN
    'disk.scsiReservationCnflctsPct': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': []
    },
    # Disk SCSI Reservation Conflicts
    # Compatibility: UNKNOWN
    'disk.scsiReservationConflicts': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': []
    },
    # Disk Throughput Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.throughput.contention': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Disk Throughput Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.throughput.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Command latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.totalLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystemDatastore']
    },
    # Read latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.totalReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Write latency
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.totalWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Usage
    # Compatibility: UNKNOWN
    'disk.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Write rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'disk.write': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'Datastore']
    },
}


HBR_METRICS = {
    #
    # Compatibility: 5.0.0
    'hbr.hbrNetRx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    #
    # Compatibility: 5.0.0
    'hbr.hbrNetTx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    #
    # Compatibility: 5.0.0
    'hbr.hbrNumVms': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


MANAGEMENTAGENT_METRICS = {
    # CPU usage
    # Compatibility: UNKNOWN
    'managementAgent.cpuUsage': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': []
    },
    # Memory used
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0
    'managementAgent.memUsed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Memory swap in
    # Compatibility: 3.5.0 / 4.1.0
    'managementAgent.swapIn': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Memory swap out
    # Compatibility: 3.5.0 / 4.1.0
    'managementAgent.swapOut': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Memory swap used
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0
    'managementAgent.swapUsed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


MEM_METRICS = {
    # Active
    # Compatibility: UNKNOWN
    'mem.active': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Active write
    # Compatibility: 4.1.0 / 5.0.0
    'mem.activewrite': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Memory Capacity Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.capacity.contention': {
        's_type': 'rate',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Memory Capacity Entitlement
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.capacity.entitlement': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Memory Capacity Provisioned
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.capacity.provisioned': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # Memory Capacity Usable
    # Compatibility: UNKNOWN
    'mem.capacity.usable': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Capacity Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.capacity.usage': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    #
    # Compatibility: UNKNOWN
    'mem.capacity.usage.userworld': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    #
    # Compatibility: UNKNOWN
    'mem.capacity.usage.vm': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Capacity Usage by VM overhead
    # Compatibility: UNKNOWN
    'mem.capacity.usage.vmOvrhd': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Capacity Usage by VMkernel Overhead
    # Compatibility: UNKNOWN
    'mem.capacity.usage.vmkOvrhd': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Compressed
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.compressed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Compression rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.compressionRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Consumed
    # Compatibility: UNKNOWN
    'mem.consumed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Memory Consumed by userworlds
    # Compatibility: UNKNOWN
    'mem.consumed.userworlds': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Consumed by VMs
    # Compatibility: UNKNOWN
    'mem.consumed.vms': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Decompression rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.decompressionRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Entitlement
    # Compatibility: 5.0.0
    'mem.entitlement': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Granted
    # Compatibility: UNKNOWN
    'mem.granted': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Heap
    # Compatibility: UNKNOWN
    'mem.heap': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Heap free
    # Compatibility: UNKNOWN
    'mem.heapfree': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Latency
    # Compatibility: 5.0.0
    'mem.latency': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swap in from host cache
    # Compatibility: UNKNOWN
    'mem.llSwapIn': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Swap in rate from host cache
    # Compatibility: 5.0.0
    'mem.llSwapInRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swap out to host cache
    # Compatibility: UNKNOWN
    'mem.llSwapOut': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Swap out rate to host cache
    # Compatibility: 5.0.0
    'mem.llSwapOutRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Host cache used for swapping
    # Compatibility: UNKNOWN
    'mem.llSwapUsed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Low free threshold
    # Compatibility: 5.0.0
    'mem.lowfreethreshold': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Worst case allocation
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.mementitlement': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'latest',
        'entity': ['ResourcePool']
    },
    # Overhead
    # Compatibility: UNKNOWN
    'mem.overhead': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Reserved overhead
    # Compatibility: 4.1.0 / 5.0.0
    'mem.overheadMax': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Overhead touched
    # Compatibility: 5.0.0
    'mem.overheadTouched': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Reserved capacity
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.reservedCapacity': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Memory Reserved capacity by userworlds
    # Compatibility: UNKNOWN
    'mem.reservedCapacity.userworld': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Reserved capacity by VMs
    # Compatibility: UNKNOWN
    'mem.reservedCapacity.vm': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Reserved capacity by VM overhead
    # Compatibility: UNKNOWN
    'mem.reservedCapacity.vmOvhd': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Reserved capacity by VMkernel Overhead
    # Compatibility: UNKNOWN
    'mem.reservedCapacity.vmkOvrhd': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Memory Reserved Capacity %
    # Compatibility: UNKNOWN
    'mem.reservedCapacityPct': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': []
    },
    # Shared
    # Compatibility: UNKNOWN
    'mem.shared': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Shared common
    # Compatibility: UNKNOWN
    'mem.sharedcommon': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # State
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'mem.state': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Swap in
    # Compatibility: UNKNOWN
    'mem.swapin': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swap in rate
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'mem.swapinRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swap out
    # Compatibility: UNKNOWN
    'mem.swapout': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swap out rate
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'mem.swapoutRate': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Swapped
    # Compatibility: UNKNOWN
    'mem.swapped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'ResourcePool']
    },
    # Swap target
    # Compatibility: UNKNOWN
    'mem.swaptarget': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Swap unreserved
    # Compatibility: UNKNOWN
    'mem.swapunreserved': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': []
    },
    # Swap used
    # Compatibility: UNKNOWN
    'mem.swapused': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Used by VMkernel
    # Compatibility: UNKNOWN
    'mem.sysUsage': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Total capacity
    # Compatibility: 4.1.0 / 5.0.0
    'mem.totalCapacity': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Total
    # Compatibility: UNKNOWN
    'mem.totalmb': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'average',
        'entity': []
    },
    # Unreserved
    # Compatibility: UNKNOWN
    'mem.unreserved': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Usage
    # Compatibility: UNKNOWN
    'mem.usage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Balloon
    # Compatibility: UNKNOWN
    'mem.vmmemctl': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Balloon target
    # Compatibility: UNKNOWN
    'mem.vmmemctltarget': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Trailing average of the ratio of capacity misses to compulsory misses for
    # the VMFS PB Cache
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.capMissRatio': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['hostsystem']
    },
    # amount of vmfs heap used by vmfs cache
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.overhead': {
        's_type': 'absolute',
        'unit': 'kilobytes',
        'rollup': 'latest',
        'entity': ['hostsystem']
    },
    # Space used for VMFS pointer blocks
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.size': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Max Space used for VMFS pointer blocks
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.sizeMax': {
        's_type': 'absolute',
        'unit': 'megaBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # File blocks with addresses cached
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.workingSet': {
        's_type': 'absolute',
        'unit': 'teraBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Max File blocks with addresses cached
    # Compatibility: 6.0.0
    'mem.vmfs.pbc.workingSetMax': {
        's_type': 'absolute',
        'unit': 'teraBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Zero
    # Compatibility: 6.0.0
    'mem.zero': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Memory saved by zipping
    # Compatibility: 4.1.0 / 5.0.0
    'mem.zipSaved': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Zipped memory
    # Compatibility: 4.1.0 / 5.0.0
    'mem.zipped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
}


NETWORK_METRICS = {
    # Broadcast receives
    # Compatibility: 5.0.0
    'network.broadcastRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Broadcast transmits
    # Compatibility: 5.0.0
    'network.broadcastTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data receive rate
    # Compatibility: 5.0.0
    'network.bytesRx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data transmit rate
    # Compatibility: 5.0.0
    'network.bytesTx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Receive packets dropped
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'network.droppedRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Transmit packets dropped
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'network.droppedTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packet receive errors
    # Compatibility: 5.0.0
    'network.errorsRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Packet transmit errors
    # Compatibility: 5.0.0
    'network.errorsTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Multicast receives
    # Compatibility: 5.0.0
    'network.multicastRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Multicast transmits
    # Compatibility: 5.0.0
    'network.multicastTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packets received
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.packetsRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packets transmitted
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.packetsTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data receive rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.received': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # vNic Throughput Contention
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.throughput.contention': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['ResourcePool']
    },
    # pNic Packets Received and Transmitted per Second
    # Compatibility: UNKNOWN
    'network.throughput.packetsPerSec': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Provisioned
    # Compatibility: UNKNOWN
    'network.throughput.provisioned': {
        's_type': 'absolute',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usable
    # Compatibility: UNKNOWN
    'network.throughput.usable': {
        's_type': 'absolute',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # vNic Throughput Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.throughput.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # pNic Throughput Usage for FT
    # Compatibility: UNKNOWN
    'network.throughput.usage.ft': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for HBR
    # Compatibility: UNKNOWN
    'network.throughput.usage.hbr': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for iSCSI
    # Compatibility: UNKNOWN
    'network.throughput.usage.iscsi': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for NFS
    # Compatibility: UNKNOWN
    'network.throughput.usage.nfs': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for VMs
    # Compatibility: UNKNOWN
    'network.throughput.usage.vm': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for vMotion
    # Compatibility: UNKNOWN
    'network.throughput.usage.vmotion': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # Data transmit rate
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'network.transmitted': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Unknown protocol frames
    # Compatibility: 5.0.0
    'network.unknownProtos': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Usage
    # Compatibility: UNKNOWN
    'network.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Compatibility: 6.0.0
    'net.broadcastRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Broadcast transmits
    # Compatibility: 6.0.0
    'net.broadcastTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data receive rate
    # Compatibility: 6.0.0
    'net.bytesRx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data transmit rate
    # Compatibility: 6.0.0
    'net.bytesTx': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Receive packets dropped
    # Compatibility: 6.0.0
    'net.droppedRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Transmit packets dropped
    # Compatibility: 4.0.0 / 4.1.0 / 6.0.0
    'net.droppedTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packet receive errors
    # Compatibility: 6.0.0
    'net.errorsRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Packet transmit errors
    # Compatibility: 6.0.0
    'net.errorsTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Multicast receives
    # Compatibility: 6.0.0
    'net.multicastRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Multicast transmits
    # Compatibility: 6.0.0
    'net.multicastTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packets received
    # Compatibility: 6.0.0
    'net.packetsRx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Packets transmitted
    # Compatibility: 6.0.0
    'net.packetsTx': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Data receive rate
    # Compatibility: 6.0.0
    'net.received': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # vNic Throughput Contention
    # Compatibility: 6.0.0
    'net.throughput.contention': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['ResourcePool']
    },
    # pNic Packets Received and Transmitted per Second
    # Compatibility: 6.0.0
    'net.throughput.packetsPerSec': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Provisioned
    # Compatibility: 6.0.0
    'net.throughput.provisioned': {
        's_type': 'absolute',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usable
    # Compatibility: 6.0.0
    'net.throughput.usable': {
        's_type': 'absolute',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # vNic Throughput Usage
    # Compatibility: 6.0.0
    'net.throughput.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['ResourcePool']
    },
    # pNic Throughput Usage for FT
    # Compatibility: 6.0.0
    'net.throughput.usage.ft': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for HBR
    # Compatibility: 6.0.0
    'net.throughput.usage.hbr': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for iSCSI
    # Compatibility: 6.0.0
    'net.throughput.usage.iscsi': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for NFS
    # Compatibility: 6.0.0
    'net.throughput.usage.nfs': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for VMs
    # Compatibility: 6.0.0
    'net.throughput.usage.vm': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # pNic Throughput Usage for vMotion
    # Compatibility: 6.0.0
    'net.throughput.usage.vmotion': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # Data transmit rate
    # Compatibility: 6.0.0
    'net.transmitted': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Unknown protocol frames
    # Compatibility: 6.0.0
    'net.unknownProtos': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['HostSystem']
    },
    # Usage
    # Compatibility: UNKNOWN
    'net.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem']
    },
}


POWER_METRICS = {
    # Host Power Capacity Usable
    # Compatibility: UNKNOWN
    'power.capacity.usable': {
        's_type': 'absolute',
        'unit': 'watt',
        'rollup': 'average',
        'entity': []
    },
    # Power Capacity Usage
    # Compatibility: UNKNOWN
    'power.capacity.usage': {
        's_type': 'absolute',
        'unit': 'watt',
        'rollup': 'average',
        'entity': []
    },
    # Host Power Capacity Provisioned
    # Compatibility: UNKNOWN
    'power.capacity.usagePct': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': []
    },
    # Energy usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'power.energy': {
        's_type': 'delta',
        'unit': 'joule',
        'rollup': 'summation',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Usage
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'power.power': {
        's_type': 'absolute',
        'unit': 'watt',
        'rollup': 'average',
        'entity': ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    # Cap
    # Compatibility: 4.1.0 / 5.0.0
    'power.powerCap': {
        's_type': 'absolute',
        'unit': 'watt',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


RESCPU_METRICS = {
    # Active (1 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actav1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Active (15 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actav15': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Active (5 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actav5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Active (1 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actpk1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Active (15 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actpk15': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Active (5 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.actpk5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Throttled (1 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.maxLimited1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Throttled (15 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.maxLimited15': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Throttled (5 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.maxLimited5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (1 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runav1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (15 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runav15': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (5 min. average)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runav5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (1 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runpk1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (15 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runpk15': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Running (5 min. peak)
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.runpk5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Group CPU sample count
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.sampleCount': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Group CPU sample period
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'rescpu.samplePeriod': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
}


STORAGEADAPTER_METRICS = {
    # Storage Adapter Outstanding I/Os
    # Compatibility: UNKNOWN
    'storageAdapter.OIOsPct': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'average',
        'entity': []
    },
    # Average commands issued per second
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.commandsAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Highest latency
    # Compatibility: 5.0.0
    'storageAdapter.maxTotalLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Average read requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.numberReadAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Average write requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.numberWriteAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Storage Adapter Outstanding I/Os
    # Compatibility: UNKNOWN
    'storageAdapter.outstandingIOs': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # Storage Adapter Queue Depth
    # Compatibility: UNKNOWN
    'storageAdapter.queueDepth': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # Storage Adapter Queue Command Latency
    # Compatibility: UNKNOWN
    'storageAdapter.queueLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': []
    },
    # Storage Adapter Number Queued
    # Compatibility: UNKNOWN
    'storageAdapter.queued': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'average',
        'entity': []
    },
    # Read rate
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.read': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Storage Adapter Throughput Contention
    # Compatibility: UNKNOWN
    'storageAdapter.throughput.cont': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': []
    },
    # Storage Adapter Throughput Usage
    # Compatibility: UNKNOWN
    'storageAdapter.throughput.usag': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # Read latency
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.totalReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Write latency
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.totalWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Write rate
    # Compatibility: 4.1.0 / 5.0.0
    'storageAdapter.write': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


STORAGEPATH_METRICS = {
    # Storage Path Bus Resets
    # Compatibility: UNKNOWN
    'storagePath.busResets': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': []
    },
    # Storage Path Command Aborts
    # Compatibility: UNKNOWN
    'storagePath.commandsAborted': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': []
    },
    # Average commands issued per second
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.commandsAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Highest latency
    # Compatibility: 5.0.0
    'storagePath.maxTotalLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Average read requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.numberReadAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Average write requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.numberWriteAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Read rate
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.read': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Storage Path Throughput Contention
    # Compatibility: UNKNOWN
    'storagePath.throughput.cont': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': []
    },
    # Storage Path Throughput Usage
    # Compatibility: UNKNOWN
    'storagePath.throughput.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # Read latency
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.totalReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Write latency
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.totalWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Write rate
    # Compatibility: 4.1.0 / 5.0.0
    'storagePath.write': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
}


SYSTEM_METRICS = {
    # Disk space usage
    # Compatibility: 4.0.0
    'system.cosDiskUsage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Disk usage
    # Compatibility: 4.1.0
    'system.diskUsage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Heartbeat
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'system.heartbeat': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # OS Uptime
    # Compatibility: 5.0.0
    'system.osUptime': {
        's_type': 'absolute',
        'unit': 'second',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Resource CPU active (1 min. average)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuAct1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU active (5 min. average)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuAct5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation maximum, in MHz
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuAllocMax': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation minimum, in MHz
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuAllocMin': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation shares
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuAllocShares': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU maximum limited (1 min.)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuMaxLimited1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU maximum limited (5 min.)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuMaxLimited5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU running (1 min. average)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuRun1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU running (5 min. average)
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceCpuRun5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU usage ({rollupType})
    # Compatibility: UNKNOWN
    'system.resourceCpuUsage': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Resource memory allocation maximum, in KB
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemAllocMax': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation minimum, in KB
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemAllocMin': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation shares
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemAllocShares': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory shared
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemCow': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory mapped
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemMapped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory overhead
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemOverhead': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory share saved
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemShared': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory swapped
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemSwapped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory touched
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemTouched': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory zero
    # Compatibility: 4.0.0 / 4.1.0 / 5.0.0
    'system.resourceMemZero': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Uptime
    # Compatibility: 3.5.0 / 4.0.0 / 4.1.0 / 5.0.0
    'system.uptime': {
        's_type': 'absolute',
        'unit': 'second',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
    # Compatibility: 6.0.0
    'sys.cosDiskUsage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Disk usage
    # Compatibility: 6.0.0
    'sys.diskUsage': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Heartbeat
    # Compatibility: 6.0.0
    'sys.heartbeat': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': ['VirtualMachine']
    },
    # OS Uptime
    # Compatibility: 6.0.0
    'sys.osUptime': {
        's_type': 'absolute',
        'unit': 'second',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Resource CPU active (1 min. average)
    # Compatibility: 6.0.0
    'sys.resourceCpuAct1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU active (5 min. average)
    # Compatibility: 6.0.0
    'sys.resourceCpuAct5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation maximum, in MHz
    # Compatibility: 6.0.0
    'sys.resourceCpuAllocMax': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation minimum, in MHz
    # Compatibility: 6.0.0
    'sys.resourceCpuAllocMin': {
        's_type': 'absolute',
        'unit': 'megaHertz',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU allocation shares
    # Compatibility: 6.0.0
    'sys.resourceCpuAllocShares': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU maximum limited (1 min.)
    # Compatibility: 6.0.0
    'sys.resourceCpuMaxLimited1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU maximum limited (5 min.)
    # Compatibility: 6.0.0
    'sys.resourceCpuMaxLimited5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU running (1 min. average)
    # Compatibility: 6.0.0
    'sys.resourceCpuRun1': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU running (5 min. average)
    # Compatibility: 6.0.0
    'sys.resourceCpuRun5': {
        's_type': 'absolute',
        'unit': 'percent',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource CPU usage ({rollupType})
    # Compatibility: 6.0.0
    'sys.resourceCpuUsage': {
        's_type': 'rate',
        'unit': 'megaHertz',
        'rollup': 'average',
        'entity': ['HostSystem']
    },
    # Resource FD usage ({rollupType})
    # Compatibility: 6.0.0
    'sys.resourceFdUsage': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation maximum, in KB
    # Compatibility: 6.0.0
    'sys.resourceMemAllocMax': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation minimum, in KB
    # Compatibility: 6.0.0
    'sys.resourceMemAllocMin': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation shares
    # Compatibility: 6.0.0
    'sys.resourceMemAllocShares': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory allocation maximum, in KB
    # Compatibility: 6.0.0
    'sys.resourceMemConsumed': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory shared
    # Compatibility: 6.0.0
    'sys.resourceMemCow': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory mapped
    # Compatibility: 6.0.0
    'sys.resourceMemMapped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory overhead
    # Compatibility: 6.0.0
    'sys.resourceMemOverhead': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory share saved
    # Compatibility: 6.0.0
    'sys.resourceMemShared': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory swapped
    # Compatibility: 6.0.0
    'sys.resourceMemSwapped': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory touched
    # Compatibility: 6.0.0
    'sys.resourceMemTouched': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Resource memory zero
    # Compatibility: 6.0.0
    'sys.resourceMemZero': {
        's_type': 'absolute',
        'unit': 'kiloBytes',
        'rollup': 'latest',
        'entity': ['HostSystem']
    },
    # Compatibility: 6.0.0
    'sys.uptime': {
        's_type': 'absolute',
        'unit': 'second',
        'rollup': 'latest',
        'entity': ['VirtualMachine', 'HostSystem']
    },
}


VIRTUALDISK_METRICS = {
    # Virtual Disk Bus Resets
    # Compatibility: UNKNOWN
    'virtualDisk.busResets': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': []
    },
    # Virtual Disk Command Aborts
    # Compatibility: UNKNOWN
    'virtualDisk.commandsAborted': {
        's_type': 'delta',
        'unit': 'number',
        'rollup': 'summation',
        'entity': []
    },
    # Average read requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.numberReadAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Average write requests per second
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.numberWriteAveraged': {
        's_type': 'rate',
        'unit': 'number',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Read rate
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.read': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Read workload metric
    # Compatibility: 5.0.0
    'virtualDisk.readLoadMetric': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Average number of outstanding read requests
    # Compatibility: 5.0.0
    'virtualDisk.readOIO': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Virtual Disk Throughput Contention
    # Compatibility: UNKNOWN
    'virtualDisk.throughput.cont': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': []
    },
    # Virtual Disk Throughput Usage
    # Compatibility: UNKNOWN
    'virtualDisk.throughput.usage': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': []
    },
    # Read latency
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.totalReadLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Write latency
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.totalWriteLatency': {
        's_type': 'absolute',
        'unit': 'millisecond',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Write rate
    # Compatibility: 4.1.0 / 5.0.0
    'virtualDisk.write': {
        's_type': 'rate',
        'unit': 'kiloBytesPerSecond',
        'rollup': 'average',
        'entity': ['VirtualMachine']
    },
    # Write workload metric
    # Compatibility: 5.0.0
    'virtualDisk.writeLoadMetric': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Average number of outstanding write requests
    # Compatibility: 5.0.0
    'virtualDisk.writeOIO': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Average number of outstanding write requests
    # Compatibility: 6.0.0
    'virtualDisk.largeSeeks': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Compatibility: 6.0.0
    'virtualDisk.mediumSeeks': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Compatibility: 6.0.0
    'virtualDisk.smallSeeks': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Compatibility: 6.0.0
    'virtualDisk.writeIOSize': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Compatibility: 6.0.0
    'virtualDisk.readIOSize': {
        's_type': 'absolute',
        'unit': 'number',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    # Compatibility: 6.0.0
    'virtualDisk.writeLatencyUS': {
        's_type': 'absolute',
        'unit': 'microsecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
    'virtualDisk.readLatencyUS': {
        's_type': 'absolute',
        'unit': 'microsecond',
        'rollup': 'latest',
        'entity': ['VirtualMachine']
    },
}


ALL_METRICS = {}
ALL_METRICS.update(CPU_METRICS)
ALL_METRICS.update(DATASTORE_METRICS)
ALL_METRICS.update(DISK_METRICS)
ALL_METRICS.update(HBR_METRICS)
ALL_METRICS.update(MANAGEMENTAGENT_METRICS)
ALL_METRICS.update(MEM_METRICS)
ALL_METRICS.update(NETWORK_METRICS)
ALL_METRICS.update(POWER_METRICS)
ALL_METRICS.update(RESCPU_METRICS)
ALL_METRICS.update(STORAGEADAPTER_METRICS)
ALL_METRICS.update(STORAGEPATH_METRICS)
ALL_METRICS.update(SYSTEM_METRICS)
ALL_METRICS.update(VIRTUALDISK_METRICS)
