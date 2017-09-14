# std
import datetime
import logging
from copy import deepcopy
from urlparse import urljoin


log = logging.getLogger('collector')

HEALTH_ENDPOINT = '/healthz'
CM_ENDPOINT = '/namespaces/{namespace}/configmaps'
CM_NAME = 'datadog-leader-elector'
CREATOR_ANNOTATION = 'creator'
ACQUIRE_TIME_ANNOTATION = 'acquired_time'
DEFAULT_LEASE_DURATION = 5 * 60  # seconds

class LeaderElector:
    """
    Uses the Kubernetes ConfigMap API to elect a leader among agents.
    This is based on the mechanism described here:
    https://github.com/kubernetes/kubernetes/blob/v1.7.3/pkg/client/leaderelection/leaderelection.go

    The election process goes like this:
    - all agents share the same CM name that they will try and lock
    by overriding its metadata.
    - if the CM doesn't exist or if its last refresh is too old:
      create or replace it with fresh metadata and become the leader
    - if the CM is already locked, there is already a leader agent. Then do nothing

    This process should be triggered regularly (more frequently than the expiration period).
    The leader needs to refresh its status by overriding the acquire-time label in the CM meta.

    This mechanism doesn't ensure uniqueness of the leader because of clock skew.
    A clock sync between nodes in the cluster is required to minimize this issue.
    Setting up NTP is generally enough.
    """

    def __init__(self, kubeutil):
        self.kubeutil = kubeutil
        self.apiserver_url = kubeutil.kubernetes_api_url
        self.self_namespace = kubeutil.self_namespace
        self.last_acquire_time = None
        self.lease_duration = kubeutil.leader_lease_duration or DEFAULT_LEASE_DURATION
        if not self._is_reachable():
            return

    def _is_reachable(self):
        health_url = urljoin(self.apiserver_url, HEALTH_ENDPOINT)
        try:
            self.kubeutil.retrieve_json_auth(health_url)
        except Exception as ex:
            log.error("API server is unreachable, disabling leader election. Error: %s" % str(ex))
            return False

    def try_acquire_or_refresh(self):
        """
        if this agent is leader, try and refresh the lock
        otherwise try and acquire it.
        """
        expiry_time = None
        if self.last_acquire_time:
            expiry_time = self.last_acquire_time + datetime.timedelta(seconds=self.lease_duration)

        if self.kubeutil.is_leader:
            if expiry_time and expiry_time - datetime.timedelta(seconds=30) <= datetime.datetime.utcnow():
                log.debug("Trying to refresh leader status")
                self.kubeutil.is_leader = self._try_refresh()
        else:
            if (not expiry_time) or (expiry_time <= datetime.datetime.utcnow()):
                self.kubeutil.is_leader = self._try_acquire()
                if self.kubeutil.is_leader:
                    log.info("Leader status acquired, Kubernetes events will be collected")

    def _try_acquire(self):
        """
        _try_acquire tries to acquire the CM lock and return leader status
        i.e. whether it succeeded or failed.
        note: if the CM already exists, is fresh, and the creator is the local node,
        this agent is elected leader. It means agents were re-deployed quickly
        and the expiry time is not up yet.
        """
        try:
            cm = self._get_cm()
            if not cm or self._is_lock_expired(cm):
                return self._try_lock_cm(cm)
            elif self._is_cm_mine(cm):
                return True
        except Exception as ex:
            log.error("Failed to acquire leader status: %s" % str(ex))
            return False

    def _try_refresh(self):
        """Check the lock's state and trigger a lock, a refresh, or nothing."""
        try:
            cm = self._get_cm()
            # If we're too slow it may have expired.
            # In this case act like for an acquire
            if not cm or self._is_lock_expired(cm):
                return self._try_lock_cm(cm)
            elif not self._is_cm_mine(cm):
                log.error("Tried refreshing the CM but it's not mine. Loosing leader election.")
                return False
            return self._try_refresh_cm(cm)
        except Exception as ex:
            log.error("Failed to renew leader status: %s" % str(ex))
            return False

    def _get_cm(self):
        """
        _get_cm returns the ConfigMap if it exists, None if it doesn't
        and raises an exception if several CM with the reserved name exist
        """
        try:
            cm_url = '{}/{}'.format(
                self.apiserver_url + CM_ENDPOINT.format(namespace=self.self_namespace),
                CM_NAME
            )
            cm = self.kubeutil.retrieve_json_auth(cm_url).json()
        except Exception as ex:
            if ex.response.status_code == 404:
                return None
            log.error("Failed to get config map %s. Error: %s" % (CM_NAME, str(ex)))
            return
        if not cm:
            return None
        else:
            acquired_time = cm['metadata'].get('annotations', {}).get(ACQUIRE_TIME_ANNOTATION)
            self.last_acquire_time = datetime.datetime.strptime(acquired_time, "%Y-%m-%dT%H:%M:%S.%f")
            return cm

    def _is_lock_expired(self, cm):
        acquired_time = cm.get('metadata', {}).get('annotations', {}).get(ACQUIRE_TIME_ANNOTATION)

        if not acquired_time:
            log.warning("acquired-time wasn't set correctly for the leader lock. Assuming"
                " it's expired so we can reset it correctly.")
            return True

        acquired_time = datetime.datetime.strptime(acquired_time, "%Y-%m-%dT%H:%M:%S.%f")

        if acquired_time + datetime.timedelta(seconds=self.lease_duration) <= datetime.datetime.utcnow():
            return True
        return False

    def _try_lock_cm(self, cm):
        """
        Try and lock the ConfigMap in 2 steps:
            - delete it
            - post the new cm as a replacement. If the post failed,
              a concurrent agent won the race and we're not leader
        """
        create_pl = self._build_create_cm_payload()
        cm_url = self.apiserver_url + CM_ENDPOINT.format(namespace=self.self_namespace)
        if cm:
            try:
                del_url = '{}/{}'.format(cm_url, cm['metadata']['name'])
                self.kubeutil.delete_to_apiserver(del_url)
            except Exception as ex:
                if ex.response.status_code != 404:  # 404 means another agent removed it already
                    log.error("Couldn't delete config map %s. Error: %s" % (cm['metadata']['name'], str(ex)))
                    return False

        try:
            self.kubeutil.post_json_to_apiserver(cm_url, create_pl)
        except Exception as ex:
            if ex.response.reason in ['AlreadyExists', 'Conflict']:
                log.debug("ConfigMap lock '%s' already exists, another agent "
                    "acquired it." % ex.response.json().get('details', {}).get('name', ''))
                return False
            else:
                log.error("Failed to post the ConfigMap lock. Error: %s" % str(ex))
                return False

        acquired_time = create_pl['metadata']['annotations'][ACQUIRE_TIME_ANNOTATION]
        self.last_acquire_time = datetime.datetime.strptime(acquired_time, "%Y-%m-%dT%H:%M:%S.%f")
        return True

    def _try_refresh_cm(self, cm):
        """Builds the updated CM payload and tries to PUT it"""
        update_pl = self._build_update_cm_payload(cm)
        cm_url = '{}/{}'.format(
            self.apiserver_url + CM_ENDPOINT.format(namespace=self.self_namespace),
            CM_NAME
        )
        try:
            self.kubeutil.put_json_to_apiserver(cm_url, update_pl)
        except Exception as ex:
            log.error("Failed to update the ConfigMap lock. Error: %s" % str(ex))
            return False

        acquired_time = update_pl['metadata']['annotations'][ACQUIRE_TIME_ANNOTATION]
        self.last_acquire_time = datetime.datetime.strptime(acquired_time, "%Y-%m-%dT%H:%M:%S.%f")
        return True

    def _build_create_cm_payload(self):
        now = datetime.datetime.utcnow()
        pl = {
            'data': {},
            'metadata': {
                'annotations': {
                    CREATOR_ANNOTATION: self.kubeutil.get_node_info()[1],  # node name
                    ACQUIRE_TIME_ANNOTATION: datetime.datetime.strftime(now, "%Y-%m-%dT%H:%M:%S.%f")
                },
                'name': CM_NAME,
                'namespace': self.self_namespace
            }
        }
        return pl

    def _build_update_cm_payload(self, cm):
        """
        Starts with the ConfigMap we got, updates the ACQUIRE_TIME
        and removes internal k8s fields
        """
        now = datetime.datetime.utcnow()
        pl = deepcopy(cm)
        del pl['metadata']['resourceVersion']
        del pl['metadata']['selfLink']
        del pl['metadata']['uid']
        pl['metadata']['annotations'][ACQUIRE_TIME_ANNOTATION] = datetime.datetime.strftime(now, "%Y-%m-%dT%H:%M:%S.%f")
        return pl

    def _is_cm_mine(self, cm):
        return cm['metadata']['annotations'].get(CREATOR_ANNOTATION) == self.kubeutil.get_node_info()[1]
