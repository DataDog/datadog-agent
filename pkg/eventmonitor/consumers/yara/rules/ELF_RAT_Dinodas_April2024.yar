
rule ELF_RAT_Dinodas_April2024 {
    meta:
        Description = "Detects Linux Variant of Dinodas RAT"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Reference = "https://securelist.com/dinodasrat-linux-implant/112284/"
        File_Hash_1 = "15412d1a6b7f79fad45bcd32cf82f9d651d9ccca082f98a0cca3ad5335284e45"
        File_Hash_2 = "bf830191215e0c8db207ea320d8e795990cf6b3e6698932e6e0c9c0588fc9eff"
        date = "2024-04-01"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "8138f1af1dc51cde924aa2360f12d650"
        yarahub_uuid = "3262efcd-9716-4030-8afa-a4e4c8d71c54"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.dinodas_rat"

    strings: 
        $key1 = {A1 A1 18 AA 10 F0 FA 16 06 71 B3 08 AA AF 31 A1} // For C2 encryption
        $key2 = {A0 21 A1 FA 18 E0 C1 30 1F 9F C0 A1 A0 A6 6F B1} // For name encryption

        $c2 = "update.centos-yum.com:443" fullword     // Hardcoded C2 domain

        $etc1 = "/etc/rc.d/rc.local" fullword
        $etc2 = "/etc/init.d/%s" fullword
        $etc3 = "cat /proc/version" fullword
        $etc4 = "cat /etc/lsb-release" fullword
        $etc5 = "/etc/rc.local" fullword

        $chck1 = "chkconfig --add %s" fullword
        $chck2 = "chkconfig zentao %s" fullword
        $chck3 = "whereis chkconfig" fullword
        $chck4 = "chkconfig --list" fullword
        $chck5 = "chkconfig --del %s" fullword

        $cmd1 = "kill %u" fullword
        $cmd2 = "kill %s" fullword
        $cmd3 = "rm -rf" fullword
        $cmd4 = "getconf LONG_BIT" fullword
        $cmd5 = "ls -l /proc/%s/exe" fullword
        $cmd6 = "service %s start" fullword
        $cmd7 = "service %s stop" fullword
        $cmd8 = "nslookup %s %s" fullword

        $info1 = "ExecStart=/etc/rc.local start" fullword
        $info2 = "After=network.target" fullword
        $info3 = "Default-Start: 2 3 4 5" fullword
        $info4 = "Default-Stop: 0 1 6" fullword
        $info5 = "multi-user.target" fullword

    condition:
        uint32be(0) == 0x7F454C46  //ELF Header
        and any of ($key*)

        and ($c2 or 
        (any of ($chck*) 
        and 2 of ($etc*)
        and 3 of ($cmd*)
        and 2 of ($info*)))

        and filesize < 1MB
 }










