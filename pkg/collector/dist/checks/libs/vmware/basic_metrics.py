# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

BASIC_METRICS = {
    'cpu.extra': {
        's_type'       : 'delta',
        'unit'         : 'millisecond',
        'rollup'       : 'summation',
        'entity'       : ['VirtualMachine']
    },
    'cpu.ready': {
        's_type'       : 'delta',
        'unit'         : 'millisecond',
        'rollup'       : 'summation',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
    'cpu.usage': {
        's_type'       : 'rate',
        'unit'         : 'percent',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
    'cpu.usagemhz': {
        's_type'       : 'rate',
        'unit'         : 'megaHertz',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'disk.commandsAborted': {
        's_type'       : 'delta',
        'unit'         : 'number',
        'rollup'       : 'summation',
        'entity'       : ['VirtualMachine', 'HostSystem', 'Datastore']
    },
    'disk.deviceLatency': {
        's_type'       : 'absolute',
        'unit'         : 'millisecond',
        'rollup'       : 'average',
        'entity'       : ['HostSystem']
    },
    'disk.deviceReadLatency': {
        's_type'       : 'absolute',
        'unit'         : 'millisecond',
        'rollup'       : 'average',
        'entity'       : ['HostSystem']
    },
    'disk.deviceWriteLatency': {
        's_type'       : 'absolute',
        'unit'         : 'millisecond',
        'rollup'       : 'average',
        'entity'       : ['HostSystem']
    },
    'disk.queueLatency': {
        's_type'       : 'absolute',
        'unit'         : 'millisecond',
        'rollup'       : 'average',
        'entity'       : ['HostSystem']
    },
    'disk.totalLatency': {
        's_type'       : 'absolute',
        'unit'         : 'millisecond',
        'rollup'       : 'average',
        'entity'       : ['HostSystemDatastore']
    },
    'mem.active': {
        's_type'       : 'absolute',
        'unit'         : 'kiloBytes',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'mem.compressed': {
        's_type'       : 'absolute',
        'unit'         : 'kiloBytes',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'mem.consumed': {
        's_type'       : 'absolute',
        'unit'         : 'kiloBytes',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'mem.overhead': {
        's_type'       : 'absolute',
        'unit'         : 'kiloBytes',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'mem.vmmemctl': {
        's_type'       : 'absolute',
        'unit'         : 'kiloBytes',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem', 'ResourcePool']
    },
    'network.received': {
        's_type'       : 'rate',
        'unit'         : 'kiloBytesPerSecond',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
    'network.transmitted': {
        's_type'       : 'rate',
        'unit'         : 'kiloBytesPerSecond',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
    'net.received': {
        's_type'       : 'rate',
        'unit'         : 'kiloBytesPerSecond',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
    'net.transmitted': {
        's_type'       : 'rate',
        'unit'         : 'kiloBytesPerSecond',
        'rollup'       : 'average',
        'entity'       : ['VirtualMachine', 'HostSystem']
    },
}
