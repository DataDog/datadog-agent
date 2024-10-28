import sys
from functools import partial

from board import finish, setup
from test_builder import LinuxConfig, MacConfig, Suite, WindowsConfig
from test_cases.containers import (
    AgentUsesAdLabels,
    ContainerCollectAll,
    ContainerScenario,
    ContainerTailJounald,
    DockerFileTail,
    DockerFileTailingAD,
    DockerMaxFile,
    PodmanFileTail,
    PodmanSocketTail,
)
from test_cases.kubernetes import (
    K8CollectAll,
    K8CollectAllDocker,
    K8DockerContainerLabels,
    K8FileTailingAnnotation,
    K8PodAnnotation,
)
from test_cases.linux import SNMPTraps, TailJounald, TailJournaldStartPosition
from test_cases.misc import Serverless, StreamLogs
from test_cases.windows import TestEventLog
from test_cases.xplat.config import DualShipping, EndpointTests
from test_cases.xplat.file_tests import (
    TailFile,
    TailFileMultiLine,
    TailFileStartPosition,
    TailFileUTF16,
    TailFileWildcard,
)
from test_cases.xplat.network import TailTCPUDP

if len(sys.argv) < 2:
    print("Usage: python release_qa.py <AGENT_VERSON>")
    exit(1)
version = sys.argv[1]

board = setup(version)

# Test that apply to all host platforms
xplatHostTests = [
    TailFile,
    TailFileWildcard,
    TailFileStartPosition,
]

misc = board.add_list("Misc (any platform)")
Suite(
    LinuxConfig(),
    [
        TailFileMultiLine,
        TailFileUTF16,
        TailTCPUDP,
        EndpointTests,
        DualShipping,
        Serverless,
        StreamLogs,
    ],
).build(misc.add_card)

kube = board.add_list("Kubernetes")
Suite(
    LinuxConfig(),
    [
        K8CollectAllDocker,
        K8DockerContainerLabels,
        K8CollectAll,
        K8PodAnnotation,
        K8FileTailingAnnotation,
    ],
).build(kube.add_card)

# exceptions are omitted from the generated test cases
CONTAINER_EXCEPTIONS = {
    # (k8s, cfgsource, cca, kcuf, dcuf)
    # These cases have never worked. See AML-240
    ('containerd', 'annotation', True, False, False),
    ('containerd', 'annotation', False, False, True),
    ('containerd', 'annotation', False, False, False),
}

containers = board.add_list("Container Runtimes")
Suite(
    LinuxConfig(),
    [
        ContainerTailJounald,
        ContainerCollectAll,
        AgentUsesAdLabels,
        DockerMaxFile,
        DockerFileTailingAD,
        DockerFileTail,
        PodmanFileTail,
        PodmanSocketTail,
    ]
    + [
        partial(ContainerScenario, k8s, cfgsource, cca, kcuf, dcuf)
        for k8s in ('docker', 'containerd', 'none')
        for cfgsource in ('label' if k8s == 'docker' else 'annotation', 'file', 'none')
        for cca in (True, False)
        for kcuf in (True, False)
        for dcuf in (True, False)
        if (k8s, cfgsource, cca, kcuf, dcuf) not in CONTAINER_EXCEPTIONS
    ],
).build(containers.add_card)

windows = board.add_list("Windows")
Suite(
    WindowsConfig(),
    xplatHostTests
    + [
        TestEventLog,
    ],
).build(windows.add_card)

mac = board.add_list("Mac")
Suite(MacConfig(), xplatHostTests + []).build(mac.add_card)

linux = board.add_list("Linux")
Suite(
    LinuxConfig(),
    xplatHostTests
    + [
        TailJounald,
        TailJournaldStartPosition,
        SNMPTraps,
    ],
).build(linux.add_card)

finish(board)
