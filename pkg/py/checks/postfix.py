# stdlib
import os

# project
from checks import AgentCheck
from utils.subprocess_output import get_subprocess_output

class PostfixCheck(AgentCheck):
    """This check provides metrics on the number of messages in a given postfix queue

    WARNING: the user that dd-agent runs as must have sudo access for the 'find' command
             sudo access is not required when running dd-agent as root (not recommended)

    example /etc/sudoers entry:
             dd-agent ALL=(ALL) NOPASSWD:/usr/bin/find

    YAML config options:
        "directory" - the value of 'postconf -h queue_directory'
        "queues" - the postfix mail queues you would like to get message count totals for
    """
    def check(self, instance):
        config = self._get_config(instance)

        directory = config['directory']
        queues = config['queues']
        tags = config['tags']

        self._get_queue_count(directory, queues, tags)

    def _get_config(self, instance):
        directory = instance.get('directory', None)
        queues = instance.get('queues', None)
        tags = instance.get('tags', [])
        if not queues or not directory:
            raise Exception('missing required yaml config entry')

        instance_config = {
            'directory': directory,
            'queues': queues,
            'tags': tags,
        }

        return instance_config

    def _get_queue_count(self, directory, queues, tags):
        for queue in queues:
            queue_path = os.path.join(directory, queue)
            if not os.path.exists(queue_path):
                raise Exception('%s does not exist' % queue_path)

            count = 0
            if os.geteuid() == 0:
                # dd-agent is running as root (not recommended)
                count = sum(len(files) for root, dirs, files in os.walk(queue_path))
            else:
                # can dd-agent user run sudo?
                test_sudo = os.system('setsid sudo -l < /dev/null')
                if test_sudo == 0:
                    output, _, _ = get_subprocess_output(['sudo', 'find', queue_path, '-type', 'f'], self.log)
                    count = len(output.splitlines())
                else:
                    raise Exception('The dd-agent user does not have sudo access')

            # emit an individually tagged metric
            self.gauge('postfix.queue.size', count, tags=tags + ['queue:%s' % queue, 'instance:%s' % os.path.basename(directory)])

            # these can be retrieved in a single graph statement
            # for example:
            #     sum:postfix.queue.size{instance:postfix-2,queue:incoming,host:hostname.domain.tld}
