rule HKTL_Gsocket_Config_Blob
{
    meta:
        description = "gs-netcat embedded operator config (664-byte gsnc_config structure)"
        reference = "https://github.com/hackerschoice/gsocket/blob/beta/tools/gsnc-utils.h"
        author = "Peter Gabaldon"
        date = "2026-08-23"
        severity = "high"
        note = "Dual-use but high-signal: the configured blob may contain relay, port, secret, and process name"
        yarahub_reference_link = "https://gist.github.com/PeterGabaldon/e5b9a6c4ad4e3278481fb5cb02d67a8c"
        yarahub_reference_md5 = "b9744f5ab86676b4a7f53cbb83655081"
        yarahub_uuid = "3729c398-8e80-4723-b4ae-a5bc83cd471c"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // "8xKd12TX" XOR 0x1F (field) XOR 0xAB (whole structure).
        $magic_on_disk = { 8C CC FF D0 85 86 E0 EC }
        // Same magic after the outer 0xAB pass in memory.
        $magic_in_mem = { 27 67 54 7B 2E 2D 4B 47 }

    condition:
        $magic_on_disk at (filesize - 8)
        or $magic_on_disk
        or $magic_in_mem
}
