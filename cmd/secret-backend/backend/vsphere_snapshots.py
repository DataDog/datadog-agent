from pyVmomi import vim
from pyVim.connect import SmartConnectNoSSL

class VSphereSnapshotCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super(VSphereSnapshotCheck, self).__init__(*args, **kwargs)
        self.vcenter  = self.instance.get("host")
        self.username = self.instance.get("username")
        self.password = self.instance.get("password")

    def check(self, instance):
        conn = self.make_connection()
        content = conn.RetrieveContent()
        vms = self.get_all_vms(content)

    def get_all_vms(content):
        subfolders = content.rootFolder.childEntity
        for folder in subfolders:
            children = folder.hostFolder.childEntity
            for compute in children:
                if compute.__class__.__name__ == "vim.ClusterComputeResource":


    def make_connection():
        return SmartConnectNoSSL(host=self.vcenter, user=self.username, pwd=self.password, port=443)
        