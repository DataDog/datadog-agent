# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
import types

# 3rd party
import requests

# project
from utils.proxy import get_proxy

log = logging.getLogger(__name__)

class GCE(object):
    URL = "http://169.254.169.254/computeMetadata/v1/?recursive=true"
    TIMEOUT = 0.3 # second
    SOURCE_TYPE_NAME = 'google cloud platform'
    metadata = None
    EXCLUDED_ATTRIBUTES = ["kube-env", "startup-script", "shutdown-script", "configure-sh", "sshKeys", "user-data",
    "cli-cert", "ipsec-cert", "ssl-cert", "google-container-manifest"]


    @staticmethod
    def _get_metadata(agentConfig):
        if GCE.metadata is not None:
            return GCE.metadata.copy()

        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            GCE.metadata = {}
            return {}

        try:
            r = requests.get(
                GCE.URL,
                timeout=GCE.TIMEOUT,
                headers={'Metadata-Flavor': 'Google'}
            )
            r.raise_for_status()
            GCE.metadata = r.json()
        except Exception as e:
            log.debug("Collecting GCE Metadata failed %s", str(e))
            GCE.metadata = {}

        return GCE.metadata.copy()



    @staticmethod
    def get_tags(agentConfig):
        if not agentConfig['collect_instance_metadata']:
            return None

        try:
            host_metadata = GCE._get_metadata(agentConfig)
            tags = []

            for key, value in host_metadata['instance'].get('attributes', {}).iteritems():
                if key in GCE.EXCLUDED_ATTRIBUTES:
                    continue
                tags.append("%s:%s" % (key, value))

            tags.extend(host_metadata['instance'].get('tags', []))
            tags.append('zone:%s' % host_metadata['instance']['zone'].split('/')[-1])
            tags.append('instance-type:%s' % host_metadata['instance']['machineType'].split('/')[-1])
            tags.append('internal-hostname:%s' % host_metadata['instance']['hostname'])
            tags.append('instance-id:%s' % host_metadata['instance']['id'])
            tags.append('project:%s' % host_metadata['project']['projectId'])
            tags.append('numeric_project_id:%s' % host_metadata['project']['numericProjectId'])

            GCE.metadata['hostname'] = host_metadata['instance']['hostname'].split('.')[0]

            return tags
        except Exception as e:
            log.debug("Collecting GCE tags failed %s", str(e))
            return None

    @staticmethod
    def get_hostname(agentConfig):
        try:
            host_metadata = GCE._get_metadata(agentConfig)
            hostname = host_metadata['instance']['hostname']
            if agentConfig.get('gce_updated_hostname'):
                return hostname
            else:
                return hostname.split('.')[0]
        except Exception:
            return None

    @staticmethod
    def get_host_aliases(agentConfig):
        try:
            host_metadata = GCE._get_metadata(agentConfig)
            project_id = host_metadata['project']['projectId']
            instance_name = host_metadata['instance']['hostname'].split('.')[0]
            return ['%s.%s' % (instance_name, project_id)]
        except Exception as e:
            log.debug("Collecting GCE host aliases failed %s", str(e))
            return None

class EC2(object):
    """Retrieve EC2 metadata
    """
    EC2_METADATA_HOST = "http://169.254.169.254"
    METADATA_URL_BASE = EC2_METADATA_HOST + "/latest/meta-data"
    INSTANCE_IDENTITY_URL = EC2_METADATA_HOST + "/latest/dynamic/instance-identity/document"
    TIMEOUT = 0.1  # second
    DEFAULT_PREFIXES = [u'ip-', u'domu']
    metadata = {}

    class NoIAMRole(Exception):
        """
        Instance has no associated IAM role.
        """
        pass

    @staticmethod
    def is_default(hostname):
        hostname = hostname.lower()
        for prefix in EC2.DEFAULT_PREFIXES:
            if hostname.startswith(prefix):
                return True
        return False

    @staticmethod
    def get_iam_role():
        """
        Retrieve instance's IAM role.
        Raise `NoIAMRole` when unavailable.
        """
        try:
            r = requests.get(EC2.METADATA_URL_BASE + "/iam/security-credentials/")
            r.raise_for_status()
            return r.content.strip()
        except requests.exceptions.HTTPError as e:
            log.debug("Collecting IAM Role failed %s", str(e))
            if e.response.status_code == 404:
                raise EC2.NoIAMRole()
            raise

    @staticmethod
    def get_tags(agentConfig):
        """
        Retrieve AWS EC2 tags.
        """
        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            return []

        EC2_tags = []

        try:
            iam_role = EC2.get_iam_role()
            iam_url = EC2.METADATA_URL_BASE + "/iam/security-credentials/" + unicode(iam_role)
            r = requests.get(iam_url, timeout=EC2.TIMEOUT)
            r.raise_for_status() # Fail on 404 etc
            iam_params = r.json()
            r = requests.get(EC2.INSTANCE_IDENTITY_URL, timeout=EC2.TIMEOUT)
            r.raise_for_status()
            instance_identity = r.json()
            region = instance_identity['region']

            import boto.ec2
            proxy_settings = get_proxy(agentConfig) or {}
            connection = boto.ec2.connect_to_region(
                region,
                aws_access_key_id=iam_params['AccessKeyId'],
                aws_secret_access_key=iam_params['SecretAccessKey'],
                security_token=iam_params['Token'],
                proxy=proxy_settings.get('host'), proxy_port=proxy_settings.get('port'),
                proxy_user=proxy_settings.get('user'), proxy_pass=proxy_settings.get('password')
            )

            tag_object = connection.get_all_tags({'resource-id': EC2.metadata['instance-id']})

            EC2_tags = [u"%s:%s" % (tag.name, tag.value) for tag in tag_object]
            if agentConfig.get('collect_security_groups') and EC2.metadata.get('security-groups'):
                EC2_tags.append(u"security-group-name:{0}".format(EC2.metadata.get('security-groups')))

        except EC2.NoIAMRole:
            log.warning(
                u"Unable to retrieve AWS EC2 custom tags: "
                u"an IAM role associated with the instance is required"
            )
        except Exception:
            log.exception("Problem retrieving custom EC2 tags")

        return EC2_tags

    @staticmethod
    def get_metadata(agentConfig):
        """Use the ec2 http service to introspect the instance. This adds latency if not running on EC2
        """
        # >>> import requests
        # >>> requests.get('http://169.254.169.254/latest/', timeout=1).content
        # 'dynamic\nmeta-data\nuser-data'
        # >>> requests.get('http://169.254.169.254/latest/meta-data', timeout=1).content
        # 'ami-id\nami-launch-index\nami-manifest-path\nblock-device-mapping/\nhostname\niam/\ninstance-action\ninstance-id\ninstance-type\nlocal-hostname\nlocal-ipv4\nmac\nmetrics/\nnetwork/\nplacement/\nprofile\npublic-hostname\npublic-ipv4\npublic-keys/\nreservation-id\nsecurity-groups\nservices/'
        # >>> requests.get('http://169.254.169.254/latest/meta-data/instance-id', timeout=1).content
        # 'i-deadbeef'

        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            return {}

        for k in ('instance-id', 'hostname', 'local-hostname', 'public-hostname', 'ami-id', 'local-ipv4', 'public-keys/', 'public-ipv4', 'reservation-id', 'security-groups'):
            try:
                url = EC2.METADATA_URL_BASE + "/" + unicode(k)
                r = requests.get(url, timeout=EC2.TIMEOUT)
                r.raise_for_status()
                v = r.content.strip()
                assert type(v) in (types.StringType, types.UnicodeType) and len(v) > 0, "%s is not a string" % v
                EC2.metadata[k.rstrip('/')] = v
            except Exception as e:
                log.debug("Collecting EC2 Metadata failed %s", str(e))
                pass

        return EC2.metadata.copy()

    @staticmethod
    def get_instance_id(agentConfig):
        try:
            return EC2.get_metadata(agentConfig).get("instance-id", None)
        except Exception:
            return None
