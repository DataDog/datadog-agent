rule Linux_XMR_Miner_xmra64
{
    meta:
        author = "0xFF1"
        description = "Linux XMR miner payload (xmra64) used by multi-stage cron-based droppers"
        date = "2026-02-05"
        yarahub_reference_md5 = "3d4ebdfc02146e6df1784a4ebd7621ff"
        yarahub_uuid = "5ec18f06-9675-403b-9ec0-fdf1e8444ac5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        /* Miner identifier */
        $miner1 = "xmra64" ascii

        /* Common execution locations */
        $path1 = "/dev/shm" ascii
        $path2 = "/var/tmp" ascii

    condition:
        uint32(0) == 0x464c457f and
        filesize > 1MB and filesize < 5MB and
        $miner1 and
        1 of ($path*)
}
