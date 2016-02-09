# stdlib
from collections import namedtuple
import time

# 3p
import paramiko

# project
from checks import AgentCheck


class CheckSSH(AgentCheck):

    OPTIONS = [
        ('host', True, None, str),
        ('port', False, 22, int),
        ('username', True, None, str),
        ('password', False, None, str),
        ('private_key_file', False, None, str),
        ('sftp_check', False, True, bool),
        ('add_missing_keys', False, False, bool),
    ]

    Config = namedtuple('Config', [
        'host',
        'port',
        'username',
        'password',
        'private_key_file',
        'sftp_check',
        'add_missing_keys',
    ])

    def _load_conf(self, instance):
        params = []
        for option, required, default, expected_type in self.OPTIONS:
            value = instance.get(option)
            if required and (not value or type(value)) != expected_type :
                raise Exception("Please specify a valid {0}".format(option))

            if value is None or type(value) != expected_type:
                self.log.debug("Bad or missing value for {0} parameter. Using default".format(option))
                value = default

            params.append(value)
        return self.Config._make(params)

    def check(self, instance):
        conf = self._load_conf(instance)
        tags = ["instance:{0}-{1}".format(conf.host, conf.port)]

        try:
            private_key = paramiko.RSAKey.from_private_key_file(conf.private_key_file)
        except Exception:
            self.warning("Private key could not be found")
            private_key = None

        client = paramiko.SSHClient()
        if conf.add_missing_keys:
            client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.load_system_host_keys()

        exception_message = None
        #Service Availability to check status of SSH
        try:
            client.connect(conf.host, port=conf.port, username=conf.username,
                password=conf.password, pkey=private_key)
            self.service_check('ssh.can_connect', AgentCheck.OK,  tags=tags,
                message=exception_message)

        except Exception as e:
            exception_message = str(e)
            status = AgentCheck.CRITICAL
            self.service_check('ssh.can_connect', status, tags=tags,
                message=exception_message)
            if conf.sftp_check:
                self.service_check('sftp.can_connect', status, tags=tags,
                    message=exception_message)
            raise

        #Service Availability to check status of SFTP
        if conf.sftp_check:
            try:
                sftp = client.open_sftp()
                #Check response time of SFTP
                start_time = time.time()
                result = sftp.listdir('.')
                status = AgentCheck.OK
                end_time = time.time()
                time_taken = end_time - start_time
                self.gauge('sftp.response_time', time_taken, tags=tags)

            except Exception as e:
                exception_message = str(e)
                status = AgentCheck.CRITICAL

            if exception_message is None:
                exception_message = "No errors occured"

            self.service_check('sftp.can_connect', status, tags=tags,
                message=exception_message)
