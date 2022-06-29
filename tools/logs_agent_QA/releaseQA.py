import sys
from trello import TrelloClient
import os
from dotenv import load_dotenv
from testBuilder import LinuxConfig, MacConfig, Suite, WindowsConfig
from test_cases.xplat.file_tests import TailFile, TailFileMultiLine, TailFileStartPosition, TailFileUTF16, TailFileWildcard
from test_cases.xplat.network import TailTCPUDP
from test_cases.xplat.config import DualShipping, EndpointTests
from test_cases.linux import TailJounald, TailJournaldStartPosition, SNMPTraps
from test_cases.windows import TestEventLog
from test_cases.containers import ContainerCollectAll, AgentUsesAdLabels, ContainerTailJounald, DockerFileTail, DockerFileTailingAD, DockerMaxFile, PodmanFileTail, PodmanSocketTail
from test_cases.kubernetes import K8CollectAll, K8CollectAllDocker, K8DockerContainerLabels, K8FileTailingAnnotation, K8PodAnnotation
from test_cases.misc import Serverless, StreamLogs

if len(sys.argv) < 2:
    print("Usage: python releaseQA.py <AGENT_VERSON>")
    exit(1)
version = sys.argv[1]

# Setup env
load_dotenv()
api_key = os.getenv('API_KEY')
api_secret = os.getenv('API_SECRET')
org_id = os.getenv('ORG_ID')

# Setup trello 
client = TrelloClient(
    api_key=api_key,
    api_secret=api_secret,
)
board = client.add_board("[" + version + "] Logs Agent Release QA", None, org_id)

for list in board.all_lists():
    list.close()
board.add_list("Done")

# Test that apply to all host platforms
xplatHostTests = [ 
    TailFile,
    TailFileWildcard,
    TailFileStartPosition
]

misc = board.add_list("Misc (any platform)")
Suite(LinuxConfig(), [
    TailFileMultiLine,
    TailFileUTF16,
    TailTCPUDP,
    EndpointTests,
    DualShipping,
    Serverless,
    StreamLogs,
]).build(lambda name, body: misc.add_card(name, body))

kube = board.add_list("Kubernetes")
Suite(LinuxConfig(), [
    K8CollectAllDocker,
    K8DockerContainerLabels,
    K8CollectAll,
    K8PodAnnotation,
    K8FileTailingAnnotation,
]).build(lambda name, body: kube.add_card(name, body))

containers = board.add_list("Container Runtimes")
Suite(LinuxConfig(), [
    ContainerTailJounald,
    ContainerCollectAll,
    AgentUsesAdLabels,
    DockerMaxFile,
    DockerFileTailingAD,
    DockerFileTail,
    PodmanFileTail,
    PodmanSocketTail,
]).build(lambda name, body: containers.add_card(name, body))

windows = board.add_list("Windows")
Suite(WindowsConfig(), xplatHostTests + [ 
    TestEventLog
]).build(lambda name, body: windows.add_card(name, body))

mac = board.add_list("Mac")
Suite(MacConfig(), xplatHostTests + [ 
]).build(lambda name, body: mac.add_card(name, body))

linux = board.add_list("Linux")
Suite(LinuxConfig(), xplatHostTests + [ 
    TailJounald,
    TailJournaldStartPosition,
    SNMPTraps,
]).build(lambda name, body: linux.add_card(name, body))

print("Your QA board is ready: ")
print(board.url)