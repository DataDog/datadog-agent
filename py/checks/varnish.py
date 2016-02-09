# stdlib
from collections import defaultdict
import re
import xml.parsers.expat # python 2.4 compatible

# project
from checks import AgentCheck
from utils.subprocess_output import get_subprocess_output


class BackendStatus(object):
    HEALTHY = 'healthy'
    SICK = 'sick'
    ALL = (HEALTHY, SICK)

    @classmethod
    def to_check_status(cls, status):
        if status == cls.HEALTHY:
            return AgentCheck.OK
        elif status == cls.SICK:
            return AgentCheck.CRITICAL
        return AgentCheck.UNKNOWN

class Varnish(AgentCheck):
    SERVICE_CHECK_NAME = 'varnish.backend_healthy'

    # XML parsing bits, a.k.a. Kafka in Code
    def _reset(self):
        self._current_element = ""
        self._current_metric = "varnish"
        self._current_value = 0
        self._current_str = ""
        self._current_type = ""

    def _start_element(self, name, attrs):
        self._current_element = name

    def _end_element(self, name, tags):
        if name == "stat":
            m_name = self.normalize(self._current_metric)
            if self._current_type in ("a", "c"):
                self.rate(m_name, long(self._current_value), tags=tags)
            elif self._current_type in ("i", "g"):
                self.gauge(m_name, long(self._current_value), tags=tags)
            else:
                # Unsupported data type, ignore
                self._reset()
                return # don't save

            # reset for next stat element
            self._reset()
        elif name in ("ident", "name") or (name == "type" and self._current_str != "MAIN"):
            self._current_metric += "." + self._current_str

    def _char_data(self, data):
        self.log.debug("Data %s [%s]" % (data, self._current_element))
        data = data.strip()
        if len(data) > 0 and self._current_element != "":
            if self._current_element == "value":
                self._current_value = long(data)
            elif self._current_element == "flag":
                self._current_type = data
            else:
                self._current_str = data

    def check(self, instance):
        # Not configured? Not a problem.
        if instance.get("varnishstat", None) is None:
            raise Exception("varnishstat is not configured")
        tags = instance.get('tags', [])
        if tags is None:
            tags = []
        else:
            tags = list(set(tags))
        varnishstat_path = instance.get("varnishstat")
        name = instance.get('name')

        # Get version and version-specific args from varnishstat -V.
        version, use_xml = self._get_version_info(varnishstat_path)

        # Parse metrics from varnishstat.
        arg = '-x' if use_xml else '-1'
        cmd = [varnishstat_path, arg]

        if name is not None:
            cmd.extend(['-n', name])
            tags += [u'varnish_name:%s' % name]
        else:
            tags += [u'varnish_name:default']

        output, _, _ = get_subprocess_output(cmd, self.log)

        self._parse_varnishstat(output, use_xml, tags)

        # Parse service checks from varnishadm.
        varnishadm_path = instance.get('varnishadm')
        if varnishadm_path:
            secretfile_path = instance.get('secretfile', '/etc/varnish/secret')
            cmd = ['sudo', varnishadm_path, '-S', secretfile_path, 'debug.health']
            output, _, _ = get_subprocess_output(cmd, self.log)
            if output:
                self._parse_varnishadm(output)

    def _get_version_info(self, varnishstat_path):
        # Get the varnish version from varnishstat
        output, error, _ = get_subprocess_output([varnishstat_path, "-V"], self.log)

        # Assumptions regarding varnish's version
        use_xml = True
        version = 3

        m1 = re.search(r"varnish-(\d+)", output, re.MULTILINE)
        # v2 prints the version on stderr, v3 on stdout
        m2 = re.search(r"varnish-(\d+)", error, re.MULTILINE)

        if m1 is None and m2 is None:
            self.log.warn("Cannot determine the version of varnishstat, assuming 3 or greater")
            self.warning("Cannot determine the version of varnishstat, assuming 3 or greater")
        else:
            if m1 is not None:
                version = int(m1.group(1))
            elif m2 is not None:
                version = int(m2.group(1))

        self.log.debug("Varnish version: %d" % version)

        # Location of varnishstat
        if version <= 2:
            use_xml = False

        return version, use_xml

    def _parse_varnishstat(self, output, use_xml, tags=None):
        """Extract stats from varnishstat -x

        The text option (-1) is not reliable enough when counters get large.
        VBE.media_video_prd_services_01(10.93.67.16,,8080).happy18446744073709551615

        2 types of data, "a" for counter ("c" in newer versions of varnish), "i" for gauge ("g")
        https://github.com/varnish/Varnish-Cache/blob/master/include/tbl/vsc_fields.h

        Bitmaps are not supported.

        Example XML output (with `use_xml=True`)
        <varnishstat>
            <stat>
                <name>fetch_304</name>
                <value>0</value>
                <flag>a</flag>
                <description>Fetch no body (304)</description>
            </stat>
            <stat>
                <name>n_sess_mem</name>
                <value>334</value>
                <flag>i</flag>
                <description>N struct sess_mem</description>
            </stat>
            <stat>
                <type>LCK</type>
                <ident>vcl</ident>
                <name>creat</name>
                <value>1</value>
                <flag>a</flag>
                <description>Created locks</description>
            </stat>
        </varnishstat>
        """
        tags = tags or []
        # FIXME: this check is processing an unbounded amount of data
        # we should explicitly list the metrics we want to get from the check
        if use_xml:
            p = xml.parsers.expat.ParserCreate()
            p.StartElementHandler = self._start_element
            end_handler = lambda name: self._end_element(name, tags)
            p.EndElementHandler = end_handler
            p.CharacterDataHandler = self._char_data
            self._reset()
            p.Parse(output, True)
        else:
            for line in output.split("\n"):
                self.log.debug("Parsing varnish results: %s" % line)
                fields = line.split()
                if len(fields) < 3:
                    break
                name, gauge_val, rate_val = fields[0], fields[1], fields[2]
                metric_name = self.normalize(name, prefix="varnish")

                # Now figure out which value to pick
                if rate_val.lower() in ("nan", "."):
                    # col 2 matters
                    self.log.debug("Varnish (gauge) %s %d" % (metric_name, int(gauge_val)))
                    self.gauge(metric_name, int(gauge_val), tags=tags)
                else:
                    # col 3 has a rate (since restart)
                    self.log.debug("Varnish (rate) %s %d" % (metric_name, int(gauge_val)))
                    self.rate(metric_name, float(gauge_val), tags=tags)

    def _parse_varnishadm(self, output):
        """ Parse out service checks from varnishadm.

        Example output:

            Backend b0 is Sick
            Current states  good:  2 threshold:  3 window:  5
            Average responsetime of good probes: 0.000000
            Oldest                                                    Newest
            ================================================================
            -------------------------------------------------------------444 Good IPv4
            -------------------------------------------------------------XXX Good Xmit
            -------------------------------------------------------------RRR Good Recv
            ----------------------------------------------------------HHH--- Happy
            Backend b1 is Sick
            Current states  good:  2 threshold:  3 window:  5
            Average responsetime of good probes: 0.000000
            Oldest                                                    Newest
            ================================================================
            ----------------------------------------------------------HHH--- Happy

        """
        # Process status by backend.
        backends_by_status = defaultdict(list)
        backend, status, message = None, None, None
        for line in output.split("\n"):
            tokens = line.strip().split(' ')
            if len(tokens) > 0:
                if tokens[0] == 'Backend':
                    backend = tokens[1]
                    status = tokens[1].lower()
                elif tokens[0] == 'Current' and backend is not None:
                    try:
                        message = ' '.join(tokens[2:]).strip()
                    except Exception:
                        # If we can't parse a message still send a status.
                        self.log.exception('Error when parsing message from varnishadm')
                        message = ''
                    backends_by_status[status].append((backend, message))

        for status, backends in backends_by_status.iteritems():
            check_status = BackendStatus.to_check_status(status)
            for backend, message in backends:
                tags = ['backend:%s' % backend]
                self.service_check(self.SERVICE_CHECK_NAME, check_status,
                                   tags=tags, message=message)
