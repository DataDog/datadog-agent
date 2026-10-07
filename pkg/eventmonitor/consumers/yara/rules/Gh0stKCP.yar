rule Gh0stKCP
{
    meta:
        author = "Netresec"
        description = "Detects HP-Socket ARQ and KCP implementations, which are used in Gh0stKCP. Forked from @stvemillertime's KCP catchall rule."
        date = "2025-09-24"
        reference = "https://netresec.com/?b=259a5af"
        yarahub_reference_md5 = "527f59d0ab8798f59e7638282b2130f6"
        yarahub_uuid = "076cdcc7-cac9-43b3-a6c6-3c99ccfe13cb"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@netresec"
        yarahub_reference_link = "https://netresec.com/?b=259a5af"
    strings:
        $hex = { be b6 1f eb da 52 46 ba 92 33 59 db bf e6 c8 e4 }
        $a01 = "[RO] %ld bytes"
        $a02 = "recv sn=%lu"
        $a03 = "[RI] %d bytes"
        $a04 = "input ack: sn=%lu rtt=%ld rto=%ld"
        $a05 = "input psh: sn=%lu ts=%lu"
        $a06 = "input probe"
        $a07 = "input wins: %lu"
        $a08 = "rcv_nxt=%lu\\n"
        $a09 = "snd(buf=%d, queue=%d)\\n"
        $a10 = "rcv(buf=%d, queue=%d)\\n"
        $a11 = "rcvbuf"
    condition:
        uint16(0) == 0x5A4D and uint32(uint32(0x3C)) == 0x00004550 and filesize < 30MB and $hex and 5 of ($a*)
}
