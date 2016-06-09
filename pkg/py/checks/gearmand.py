# 3rd party
import gearman

# project
from checks import AgentCheck

class Gearman(AgentCheck):
    SERVICE_CHECK_NAME = 'gearman.can_connect'

    def get_library_versions(self):
        return {"gearman": gearman.__version__}

    def _get_client(self,host,port):
        self.log.debug("Connecting to gearman at address %s:%s" % (host, port))
        return gearman.GearmanAdminClient(["%s:%s" %
            (host, port)])

    def _get_metrics(self, client, tags):
        data = client.get_status()
        running = 0
        queued = 0
        workers = 0

        for stat in data:
            running += stat['running']
            queued += stat['queued']
            workers += stat['workers']

        unique_tasks = len(data)

        self.gauge("gearman.unique_tasks", unique_tasks, tags=tags)
        self.gauge("gearman.running", running, tags=tags)
        self.gauge("gearman.queued", queued, tags=tags)
        self.gauge("gearman.workers", workers, tags=tags)

        self.log.debug("running %d, queued %d, unique tasks %d, workers: %d"
        % (running, queued, unique_tasks, workers))

    def _get_conf(self, instance):
        host = instance.get('server', None)
        port = instance.get('port', None)

        if host is None:
            self.warning("Host not set, assuming 127.0.0.1")
            host = "127.0.0.1"

        if port is None:
            self.warning("Port is not set, assuming 4730")
            port = 4730

        tags = instance.get('tags', [])

        return host, port, tags

    def check(self, instance):
        self.log.debug("Gearman check start")

        host, port, tags = self._get_conf(instance)
        service_check_tags = ["server:{0}".format(host),
            "port:{0}".format(port)]

        client = self._get_client(host, port)
        self.log.debug("Connected to gearman")

        tags += service_check_tags

        try:
            self._get_metrics(client, tags)
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                message="Connection to %s:%s succeeded." % (host, port),
                tags=service_check_tags)
        except Exception as e:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                message=str(e), tags=service_check_tags)
            raise
