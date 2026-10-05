rule SUSP_ZIP_Smuggling_Jun01
{
    meta:
        description = "ZIP archives with data smuggled between last file record and the central directory."
        author = "delivr.to"
        date = "2025-06-01"
        score = 60
        reference = "https://github.com/Octoberfest7/zip_smuggling/"
        yarahub_uuid = "94a9028c-5def-4af1-bcf5-f9a026ae1a48"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c33861eb02323660ecef2f261f92880d"
    strings:
        $lfh  = { 50 4B 03 04 }            // Local File Header
        $eocd = { 50 4B 05 06 }            // End of Central Directory

    condition:
        // Zip File Header
        uint32(0) == 0x04034b50 and

        // Require at least one LFH, no more than 10 and exactly one EOCD
        #lfh > 0 and
        #lfh <= 10 and
        #eocd == 1 and

        // Extract reasonable name and extra field lengths from last LFH
        uint16(@lfh[#lfh] + 26) <= 256 and
        uint16(@lfh[#lfh] + 28) <= 256 and

        // Compressed size from LFH must be present and sane
        uint32(@lfh[#lfh] + 18) > 0 and
        uint32(@lfh[#lfh] + 18) < 100000000 and

        // CD offset must lie after the LFH (avoid malformed back-jumps)
        uint32(@eocd[1] + 16) > @lfh[#lfh] and 

        // Compute estimated end of file content:
        // offset_of_LFH + lfh_size (fixed) + name_len + extra_len + compressed_size
        // Compare with central directory offset from EOCD
        // Gap must be 64b+, accounts for nonstandard but benign padding (data descriptors etc.)
        // but sacrificing detection of small file smuggling
        (
            uint32(@eocd[1] + 16) - (
                @lfh[#lfh] +
                30 +
                uint16(@lfh[#lfh] + 26) +
                uint16(@lfh[#lfh] + 28) +
                uint32(@lfh[#lfh] + 18)
            )
        ) > 64
}
