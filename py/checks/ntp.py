# 3p
import ntplib

# project
from checks import AgentCheck
from utils.ntp import get_ntp_args, set_user_ntp_settings

DEFAULT_OFFSET_THRESHOLD = 60  # in seconds


class NtpCheck(AgentCheck):

    DEFAULT_MIN_COLLECTION_INTERVAL = 900  # in seconds

    def check(self, instance):
        service_check_msg = None
        offset_threshold = instance.get('offset_threshold', DEFAULT_OFFSET_THRESHOLD)
        try:
            offset_threshold = int(offset_threshold)
        except (TypeError, ValueError):
            raise Exception('Must specify an integer value for offset_threshold. Configured value is %s' % repr(offset_threshold))

        set_user_ntp_settings(dict(instance))

        req_args = get_ntp_args()

        self.log.debug("Using ntp host: {0}".format(req_args['host']))

        try:
            ntp_stats = ntplib.NTPClient().request(**req_args)
        except ntplib.NTPException:
            self.log.debug("Could not connect to NTP Server {0}".format(
                req_args['host']))
            status = AgentCheck.UNKNOWN
            ntp_ts = None
        else:
            ntp_offset = ntp_stats.offset

            # Use the ntp server's timestamp for the time of the result in
            # case the agent host's clock is messed up.
            ntp_ts = ntp_stats.recv_time
            self.gauge('ntp.offset', ntp_offset, timestamp=ntp_ts)

            if abs(ntp_offset) > offset_threshold:
                status = AgentCheck.CRITICAL
                service_check_msg = "Offset {0} secs higher than offset threshold ({1} secs)".format(ntp_offset, offset_threshold)
            else:
                status = AgentCheck.OK

        self.service_check('ntp.in_sync', status, timestamp=ntp_ts, message=service_check_msg)
