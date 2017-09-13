import logging
import time

log = logging.getLogger('collector')


class KubeEventRetriever:
    """
    Queries the apiserver for events of given kinds and namespaces
    and filters them on ressourceVersion to return only the new ones

    Best performance is achieved with only one namespace & one kind
    (server side-filtering), but multiple ns or kinds are supported
    via client-side filtering

    Needs a KubeUtil objet to route requests through
    Best way to get one is through KubeUtil.get_event_retriever()

    At the moment (k8s v1.3) there is no support to select events based on a timestamp query, so we
    go through the whole list every time. This should be fine for now as events
    have a TTL of one hour[1] but logic needs to improve as soon as they provide
    query capabilities or at least pagination, see [2][3].

    [1] https://github.com/kubernetes/kubernetes/blob/release-1.3.0/cmd/kube-apiserver/app/options/options.go#L51
    [2] https://github.com/kubernetes/kubernetes/issues/4432
    [3] https://github.com/kubernetes/kubernetes/issues/1362
    """

    def __init__(self, kubeutil_object, namespaces=None, kinds=None, delay=None):
        """
        :param kubeutil_object: valid, initialised KubeUtil objet to route requests through
        :param namespaces: namespace(s) to watch (string or list)
        :param kinds: kinds(s) to watch (string or list)
        :param delay: minimum time (in seconds) between two apiserver requests, return [] in the meantime
        """
        self.kubeutil = kubeutil_object
        self.last_resversion = -1
        self.set_namespaces(namespaces)
        self.set_kinds(kinds)
        self.set_delay(delay)

        self._last_lookup_timestamp = -1

    def set_namespaces(self, namespaces):
        self.request_url = self.kubeutil.kubernetes_api_url + '/events'
        self.namespace_filter = None
        if isinstance(namespaces, set) or isinstance(namespaces, list):
            if len(namespaces) == 1:
                namespaces = namespaces[0]
            else:
                # Client-side filtering
                self.namespace_filter = set(namespaces)
        if isinstance(namespaces, basestring):
            self.request_url = "%s/namespaces/%s/events" % (self.kubeutil.kubernetes_api_url, namespaces)

    def set_kinds(self, kinds):
        self.kind_filter = None
        self.request_params = {}
        if isinstance(kinds, set) or isinstance(kinds, list):
            if len(kinds) == 1:
                kinds = kinds[0]
            else:
                # Client-side filtering
                self.kind_filter = set(kinds)
        if isinstance(kinds, basestring):
            self.request_params['fieldSelector'] = 'involvedObject.kind=' + kinds

    def set_delay(self, delay):
        """Request throttling to reduce apiserver traffic"""
        self._request_interval = delay

    def get_event_array(self):
        """
        Fetch latest events from the apiserver for the namespaces and kinds set on init
        and returns an array of event objects.
        """

        # Request throttling
        if self._request_interval:
            if (time.time() - self._last_lookup_timestamp) < self._request_interval:
                return []
            else:
                self._last_lookup_timestamp = time.time()

        lastest_resversion = None
        filtered_events = []

        events = self.kubeutil.retrieve_json_auth(self.request_url, params=self.request_params).json()

        for event in events.get('items', []):
            resversion = int(event.get('metadata', {}).get('resourceVersion', None))
            if resversion > self.last_resversion:
                lastest_resversion = max(lastest_resversion, resversion)

                if self.namespace_filter is not None:
                    ns = event.get('involvedObject', {}).get('namespace', 'default')
                    if ns not in self.namespace_filter:
                        continue

                if self.kind_filter is not None:
                    kind = event.get('involvedObject', {}).get('kind', None)
                    if kind is None or kind not in self.kind_filter:
                        continue

                filtered_events.append(event)

        self.last_resversion = max(self.last_resversion, lastest_resversion)

        return filtered_events
