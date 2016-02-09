# 3rd party
import snakebite.client
import snakebite.version

# project
from checks import AgentCheck

# This is only available on snakebite >= 2.2.0
# but snakebite 2.x is only compatible with hadoop >= 2.2.0
# So we bundle snakebite 1.3.9 and let the possibility to upgrade to a newer version
# if people want to use HA Mode
try:
    # FIXME: Can be remove when we upgrade pylint (drop py 2.6)
    # pylint: disable=E0611
    from snakebite.namenode import Namenode
except ImportError:
    Namenode = None


DEFAULT_PORT = 8020


class HDFSCheck(AgentCheck):
    """Report on free space and space used in HDFS.
    """

    def get_client(self, instance):

        if 'namenode' in instance:
            # backward compatibility for old style configuration of that check
            host, port = instance['namenode'], instance.get('port', DEFAULT_PORT)
            return snakebite.client.Client(host, port)

        if type(instance['namenodes']) != list or len(instance['namenodes']) == 0:
            raise ValueError('"namenodes parameter should be a list of dictionaries.')

        for namenode in instance['namenodes']:
            if type(namenode) != dict:
                raise ValueError('"namenodes parameter should be a list of dictionaries.')

            if "url" not in namenode:
                raise ValueError('Each namenode should specify a "url" parameter.')

        if len(instance['namenodes']) == 1:
            host, port = instance['namenodes'][0]['url'], instance['namenodes'][0].get('port', DEFAULT_PORT)
            return snakebite.client.Client(host, port)

        else:
            # We are running on HA mode
            if Namenode is None:
                # We are running snakebite 1.x which is not compatible with the HA mode
                # Let's display a warning and use regular mode
                self.warning("HA Mode is not available with snakebite < 2.2.0"
                    "Upgrade to the latest version of snakebiteby running: "
                    "sudo /opt/datadog-agent/embedded/bin/pip install --upgrade snakebite")

                host, port = instance['namenodes'][0]['url'], instance['namenodes'][0].get('port', DEFAULT_PORT)
                return snakebite.client.Client(host, port)
            else:
                self.log.debug("Running in HA Mode")
                nodes = []
                for namenode in instance['namenodes']:
                    nodes.append(Namenode(namenode['url'], namenode.get('port', DEFAULT_PORT)))

                return snakebite.client.HAClient(nodes)

    def check(self, instance):
        if 'namenode' not in instance and 'namenodes' not in instance:
            raise ValueError('Missing key \'namenode\' in HDFSCheck config')

        tags = instance.get('tags', None)

        hdfs = self.get_client(instance)
        stats = hdfs.df()
        # {'used': 2190859321781L,
        #  'capacity': 76890897326080L,
        #  'under_replicated': 0L,
        #  'missing_blocks': 0L,
        #  'filesystem': 'hdfs://hostname:port',
        #  'remaining': 71186818453504L,
        #  'corrupt_blocks': 0L}

        self.gauge('hdfs.used', stats['used'], tags=tags)
        self.gauge('hdfs.free', stats['remaining'], tags=tags)
        self.gauge('hdfs.capacity', stats['capacity'], tags=tags)
        self.gauge('hdfs.in_use', float(stats['used']) /
                float(stats['capacity']), tags=tags)
        self.gauge('hdfs.under_replicated', stats['under_replicated'],
                tags=tags)
        self.gauge('hdfs.missing_blocks', stats['missing_blocks'], tags=tags)
        self.gauge('hdfs.corrupt_blocks', stats['corrupt_blocks'], tags=tags)
