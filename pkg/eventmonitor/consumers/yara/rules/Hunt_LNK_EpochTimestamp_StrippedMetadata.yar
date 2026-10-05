rule Hunt_LNK_EpochTimestamp_StrippedMetadata {
    meta:
        description = "Detects LNK files with timestamps reset to Jan 1, 1970 and stripped MAC/NetBIOS metadata"
        author = "Serhii Kocherhan"
        date = "2026-08-17"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "5a26e50e-55a7-4544-bef4-bbe3c538f8db"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // LNK Header Magic Number (%LNK)
        $lnk_header = { 4C 00 00 00 01 14 02 00 00 00 00 00 C0 00 00 00 00 00 00 46 }

        // FILETIME 0x019DB1DED53E8000 = 1970-01-01 00:00:00 UTC (Little Endian)
        $ft_1970 = { 00 80 3E D5 DE B1 9D 01 }

        // TrackerDataBlock Signature (BlockSize >= 0x58, BlockSignature = 0xA0000003)
        $tracker_sig = { 03 00 00 A0 }

    condition:
        // Validate LNK Header and file size boundary
        $lnk_header at 0 and filesize < 2MB and

        // Check for 1970 FILETIME at CreationTime (0x1C), AccessTime (0x24), or WriteTime (0x2C)
        (
            $ft_1970 at 0x1C or
            $ft_1970 at 0x24 or
            $ft_1970 at 0x2C
        ) and

        // Verify stripped metadata using targeted structural checks
        (
            // Check if the file trailing DWORD is zeroed (padded/stripped ExtraData block terminator)
            uint32(filesize - 4) == 0x00000000 or

            // If a TrackerDataBlock is present, check if the MAC address node field inside the Droid volume ID is zeroed
            for any i in (1..#tracker_sig) : (
                uint32(@tracker_sig[i] + 0x10) == 0x00000000 and
                uint16(@tracker_sig[i] + 0x14) == 0x0000
            )
        )
}