import os
import sys

from dotenv import load_dotenv
from test_builder import LinuxConfig, MacConfig, Suite, WindowsConfig
from test_cases.containers import (
    AgentUsesAdLabels,
    ContainerCollectAll,
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
from trello import TrelloClient

if len(sys.argv) < 2:
    print("Usage: python release_qa.py <AGENT_VERSON>")
    exit(1)
version = sys.argv[1]

# Setup env
load_dotenv()
api_key = os.getenv('API_KEY')
api_secret = os.getenv('API_SECRET')
org_id = os.getenv('ORG_ID')

# Setup trello
client = TrelloClient(api_key=api_key, api_secret=api_secret)
board = client.add_board("[" + version + "] Logs Agent Release QA", None, org_id)

for list in board.all_lists():
    list.close()
board.add_list("Done")

# Test that apply to all host platforms
xplatHostTests = [TailFile, TailFileWildcard, TailFileStartPosition]

misc = board.add_list("Misc (any platform)")
Suite(
    LinuxConfig(), [TailFileMultiLine, TailFileUTF16, TailTCPUDP, EndpointTests, DualShipping, Serverless, StreamLogs]
).build(misc.add_card)

kube = board.add_list("Kubernetes")
Suite(
    LinuxConfig(), [K8CollectAllDocker, K8DockerContainerLabels, K8CollectAll, K8PodAnnotation, K8FileTailingAnnotation]
).build(kube.add_card)

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
    ],
).build(containers.add_card)

windows = board.add_list("Windows")
Suite(WindowsConfig(), xplatHostTests + [TestEventLog]).build(windows.add_card)

mac = board.add_list("Mac")
Suite(MacConfig(), xplatHostTests + []).build(mac.add_card)

linux = board.add_list("Linux")
Suite(LinuxConfig(), xplatHostTests + [TailJounald, TailJournaldStartPosition, SNMPTraps]).build(linux.add_card)

print("Your QA board is ready: ")
print(board.url)
