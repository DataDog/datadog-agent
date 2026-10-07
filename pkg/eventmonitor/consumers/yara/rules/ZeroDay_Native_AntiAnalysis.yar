rule ZeroDay_Native_AntiAnalysis : dropper android antianalysis
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay dropper native library - anti-emulator and anti-frida checks"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "61853ecf-ca63-468d-97b3-8e2f909f5c30"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $anti1 = "/system/bin/nox" ascii
        $anti2 = "/dev/goldfish_pi" ascii
        $anti3 = "/proc/self/maps" ascii
        $anti4 = "/proc/self/task" ascii
        $anti5 = "frida" ascii nocase
        $anti6 = "qemu" ascii nocase
        
        $jni1 = "ndpVC" ascii
        $jni2 = "nativeEnvCheck" ascii
        $jni3 = "getNativeStr" ascii
        
        $verify1 = "getPackageInfo" ascii
        $verify2 = "SHA-256" ascii
        $verify3 = "signatures" ascii
        
    condition:
        (uint32(0) == 0x464C457F) and
        filesize < 500KB and
        (
            (3 of ($anti*)) or
            (2 of ($jni*)) or
            (all of ($verify*) and any of ($anti*))
        )
}
