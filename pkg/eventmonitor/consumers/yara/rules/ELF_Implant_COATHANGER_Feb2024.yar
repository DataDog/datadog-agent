
rule ELF_Implant_COATHANGER_Feb2024 {
    meta:
        Description = "Detects COTHANGER malware that spawns a BusyBox Reverse Shell "
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Credits = "Is Now on VT! for the notification of malware sample"
        Reference = "https://www.ncsc.nl/binaries/ncsc/documenten/publicaties/2024/februari/6/mivd-aivd-advisory-coathanger-tlp-clear/TLP-CLEAR+MIVD+AIVD+Advisory+COATHANGER.pdf"
        Hash = "218a64bc50f4f82d07c459868b321ec0ef5cf315b012255a129e0bde5cc80320"
        date = "2024-02-23"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "8d0fffd6b8b127e0972e281c85fbf11c"
        yarahub_uuid = "a0b24c44-9d87-4886-b6bb-b709ab3aa67b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $etc1 = "/etc/modules/%s"
        $etc2 = "/etc/shadow"
        $etc3 = "/etc/passwd"
        $etc4 = "/etc/shells"
        $etc5 = "/etc/gshadow"
        $etc6 = "/etc/hostid"
        $etc7 = "/etc/issue"
        $etc8 = "/etc/nologin"
        $etc9 = "/etc/motd"
        $etc10 = "/etc/network/if-%s.d"
        $etc11 = "/etc/ifplugd/ifplugd.action"
        $etc12 = "/etc/mactab"


        $conf1 = "/etc/man.config"
        $conf2 = "/etc/man_db.conf"
        $conf3 = "/etc/dnsd.conf"
        $conf4 = "/etc/udhcpd.conf"
        $conf5 = "/etc/ntp.conf"
        $conf6 = "/etc/inetd.conf"

        $bsybx1 = "busybox" nocase
        $bsybx2 = "/etc/busybox.conf"
        $bsybx3 = "busybox --show SCRIPT"
        $bsybx4 = "busybox --install [-s] [DIR]"

        $cmd1 = "--setgroups=allow and --map-root-user are mutually exclusive"
        $cmd2 = "tar -zcf /var/log/bootlog.tgz header %s *.log"
        $cmd3 = "cat /var/run/udhcpc.%iface%.pid"
        $cmd4 = "test -f /var/run/udhcpc.%iface%.pid"
        $cmd5 = "run-parts /etc/network/if-%s.d"
        $cmd6 = "/var/run/ifplugd.%s.pid"
        $cmd7 = "start-stop-daemon --stop -x wvdial -p /var/run/wvdial.%iface% -s 2"

        $httprsp1 = "HTTP/1.1 %u %s"
        $httprsp2 = "Content-type: %s"
        $httprsp3 = "WWW-Authenticate: Basic realm=\"%.999s\""
        $httprsp4 = "Location: %s/%s%s"
        $httprsp5 = "Content-Range: bytes %lu-%lu/%lu"
        $httprsp6 = "Accept-Ranges: bytes"
        $httprsp7 = "ETag: %s"
        $httprsp8 = "Content-Encoding: gzip"


    condition:
        6 of ($etc*)
        and 3 of ($conf*)
        and any of ($bsybx*)
        and 4 of ($cmd*)
        and all of ($httprsp*)
     
 }


 