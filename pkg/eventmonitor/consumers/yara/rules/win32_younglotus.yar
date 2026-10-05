rule win32_younglotus {
    meta:
        author = "Reedus0"
        description = "Rule for detecting YoungLotus malware"
        date = "2024-07-08"
        yarahub_reference_link = "https://habr.com/ru/articles/827184/"
        yarahub_reference_link = "https://malpedia.caad.fkie.fraunhofer.de/details/win.younglotus"
        yarahub_reference_md5 = "74D876023652002FC403052229ADC44E"
        yarahub_uuid = "6754bc2a-adc1-4970-a04d-561098812946"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.younglotus"
        version = "2"
    strings:
        $string_0 = "%s:%d:%s"
        $string_1 = "SYSTEM\\CurrentControlSet\\Services\\"
        $string_2 = "WinSta0\\Default"
        $string_3 = "%4d-%.2d-%.2d %.2d:%.2d"
        $string_4 = "%d*%sMHz"
        $string_5 = "Win7"
        $string_6 = "Shellex"
        $string_7 = "%s%s%s%s%s%s"
        $string_8 = "AVtype_info"
    condition:
        uint16(0) == 0x5A4D and 4 of them and filesize < 300KB
}