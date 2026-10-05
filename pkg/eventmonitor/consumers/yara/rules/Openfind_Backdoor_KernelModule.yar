/*
    Openfind MailGates backdoor toolset (public family name: QuietEnvelope)
    Static analysis of the dropped ELF/Perl components. Samples were never executed.
    Reference: https://infosec.exchange/@ESETresearch/115605956103322406
*/

rule Openfind_Backdoor_KernelModule
{
    meta:
        description = "libusb.ko: kernel netfilter LOCAL_IN backdoor, runs commands as root via call_usermodehelper"
        author = "shaber"
        date = "2026-08-11"
        reference = "https://infosec.exchange/@ESETresearch/115605956103322406"
        yarahub_uuid = "c983876b-0025-4439-9812-6e79631e5e85"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f11184fd99bf3bda33c09caec7d44020"
        sha256 = "77344e88a75a53b2fde09ab0c602060a2b4ffbc77eadbd17b9cff172319eb6e9"
        note = "libusb is a userspace library; no legitimate libusb.ko exists in the kernel tree."

    strings:
        
        $trig = "EXEC_OPENFIND:"              ascii

        
        $mask = "xfsaild/dm-6"                ascii

        
        $fn1 = "smtp_backdoor_init"           ascii
        $fn2 = "smtp_backdoor_exit"           ascii
        $fn3 = "check_for_command"            ascii
        $fn4 = "ingress_hook"                 ascii

        
        $pipe = "/tmp/smtp_pipe_%d_%lu"       ascii
        $sh   = "(%s) > %s 2>&1; rm -f %s"    ascii

        
        $env  = "HOME=/root"                  ascii

    condition:
        uint32(0) == 0x464C457F
        and (
            $trig
            or ($mask and $pipe)
            or 2 of ($fn*)
            or ($sh and $env)
        )
}

