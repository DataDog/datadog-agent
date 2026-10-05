/*
    YARA Rule for detecting malicious Internet Explorer IDLIST in OLE-embedded LNKs

    Detection logic:
    1. OLE compound file (not standalone .lnk)
    2. Contains embedded LNK structure with IDLIST
    3. IDLIST contains Internet Explorer root folder GUID
    4. Subsequent IDLIST items contain suspicious strings (UNC, WebDAV, file://, etc.)
    5. Strings must be within the size boundaries of their IDLIST entry

    IDLIST Structure Reference (MS-SHLLINK):
    - LNK header: 76 bytes (0x4C)
    - IDLIST size at offset 0x4C: 2 bytes
    - IDLIST items start at offset 0x4E
    - Each item: [2-byte size][1-byte type][data...]
    - Root folder item (type 0x1F): [size=0x14][type=0x1F][sort_index][GUID=16 bytes]

    Internet Explorer CLSID: {871C5380-42A0-1069-A2EA-08002B30309D}
*/

rule OLE_LNK_InternetExplorer_IDLIST_Suspicious
{
    meta:
        author = "node5"
        description = "Detects OLE-embedded LNK with Internet Explorer IDLIST containing suspicious WebDAV/UNC/file:// strings within item boundaries"
        date = "2026-02-06"
        yarahub_author_twitter = "@node5"
        yarahub_reference_md5 = "83ebe0e9d69abebc6fe33e47d27df885"
        yarahub_uuid = "ae8b195a-0b28-4018-a6c9-e51045247ef9"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // OLE compound file magic
        $ole_magic = { D0 CF 11 E0 A1 B1 1A E1 }

        // LNK header: header size (0x4C) + ShellLink CLSID {00021401-0000-0000-C000-000000000046}
        $lnk_header = { 4C 00 00 00 01 14 02 00 00 00 00 00 C0 00 00 00 00 00 00 46 }

        // Internet Explorer root folder GUID {871C5380-42A0-1069-A2EA-08002B30309D}
        $ie_guid = { 80 53 1C 87 A0 42 69 10 A2 EA 08 00 2B 30 30 9D }

        // Suspicious strings (matches both ASCII and UTF-16LE)
        $davwwwroot = "DavWWWRoot" ascii wide nocase
        $ssl = "@SSL" ascii wide nocase
        $webdav = "webdav" ascii wide nocase
        $file_proto = "file://" ascii wide nocase
        // Static port patterns (WebDAV port smuggling)
        $port_80 = "@80/" ascii wide
        $port_443 = "@443/" ascii wide
        $port_8443 = "@8443/" ascii wide
        $port_ssl = "@SSL@" ascii wide

        // Suspicious CDN/hosting domains commonly abused
        $cdn_pages = "pages.dev" ascii wide nocase
        $cdn_workers = "workers.dev" ascii wide nocase
        $cdn_bunny = "b-cdn.net" ascii wide nocase

        // Path prefix patterns (must appear at start of item data)
        // Adding 3rd byte reduces false positives vs 2-byte patterns
        // ASCII versions - 3 bytes total
        $path_unc_a = { 5C 5C ?? }  // \\ + any byte
        $path_fwd_a = { 2F 2F ?? }  // // + any byte
        $path_mix_a = { 5C 2F ?? }  // \/ + any byte

        // Malformed/obfuscated path patterns (3+ slashes)
        $path_unc3_a = { 5C 5C 5C }  // \\\
        $path_fwd3_a = { 2F 2F 2F }  // ///
        $path_mix2_a = { 5C 2F 2F }  // \//
        $path_mix3_a = { 5C 5C 2F }  // \\/
        $path_mix4_a = { 5C 2F 5C 2F }  // \/\/

        // UTF-16LE versions - 6 bytes total
        $path_unc_w = { 5C 00 5C 00 ?? 00 }  // \\ + any wide char
        $path_fwd_w = { 2F 00 2F 00 ?? 00 }  // // + any wide char
        $path_mix_w = { 5C 00 2F 00 ?? 00 }  // \/ + any wide char

        // Malformed/obfuscated (wide)
        $path_unc3_w = { 5C 00 5C 00 5C 00 }  // \\\ (wide)
        $path_fwd3_w = { 2F 00 2F 00 2F 00 }  // /// (wide)
        $path_mix2_w = { 5C 00 2F 00 2F 00 }  // \// (wide)
        $path_mix3_w = { 5C 00 5C 00 2F 00 }  // \\/ (wide)
        $path_mix4_w = { 5C 00 2F 00 5C 00 2F 00 }  // \/\/ (wide)

        // Executable extensions (comprehensive list)
        $exe = ".exe" ascii wide nocase
        $dll = ".dll" ascii wide nocase
        $scr = ".scr" ascii wide nocase
        $bat = ".bat" ascii wide nocase
        $cmd = ".cmd" ascii wide nocase
        $ps1 = ".ps1" ascii wide nocase
        $vbs = ".vbs" ascii wide nocase
        $vbe = ".vbe" ascii wide nocase
        $js = ".js" ascii wide nocase
        $jse = ".jse" ascii wide nocase
        $wsf = ".wsf" ascii wide nocase
        $wsh = ".wsh" ascii wide nocase
        $hta = ".hta" ascii wide nocase
        $lnk = ".lnk" ascii wide nocase
        $application = ".application" ascii wide nocase
        $msi = ".msi" ascii wide nocase
        $msp = ".msp" ascii wide nocase
        $cpl = ".cpl" ascii wide nocase
        $jar = ".jar" ascii wide nocase
        $pif = ".pif" ascii wide nocase
        $gadget = ".gadget" ascii wide nocase
        $reg = ".reg" ascii wide nocase

    condition:
        // Must be OLE file (distinguishes from standalone .lnk files)
        $ole_magic at 0 and
        filesize < 50MB and
        filesize > 512 and

        // Find LNK header embedded in OLE
        for any i in (1..#lnk_header) : (
            // Get position of this LNK header match
            (@lnk_header[i] >= 0) and

            // Check HasLinkTargetIDList flag (bit 0 of LinkFlags at offset 0x14)
            (uint32(@lnk_header[i]+0x14) & 0x00000001) != 0 and

            // Read IDLIST size at offset 0x4C (must be reasonable)
            uint16(@lnk_header[i]+0x4C) > 0 and
            uint16(@lnk_header[i]+0x4C) < 4096 and
            @lnk_header[i]+0x4C+uint16(@lnk_header[i]+0x4C)+2 < filesize and

            // IDLIST items start at offset 0x4E
            // Use IE GUID positions as anchors to find items and check subsequent items
            // This automatically iterates over ALL items that contain IE GUID (handles any position in list)
            //
            // Logic:
            // 1. Find all IE GUID occurrences within this LNK's IDLIST
            // 2. For each IE GUID, work backwards to find the item start (@ie_guid - 4)
            // 3. Use the item size field to calculate next item position
            // 4. Check if next item contains suspicious strings with proper anchoring
            (
                // For each IE GUID occurrence in this IDLIST, check if next item has suspicious content
                for any j in (1..#ie_guid) : (
                    // IE GUID must be within this LNK's IDLIST bounds
                    @ie_guid[j] >= @lnk_header[i]+0x4E and
                    @ie_guid[j] < @lnk_header[i]+0x4E+uint16(@lnk_header[i]+0x4C) and

                    // IE GUID appears at offset +4 within root folder item (type 0x1F)
                    // So: item_start = @ie_guid[j] - 4
                    @ie_guid[j] >= @lnk_header[i]+0x4E+4 and
                    uint8(@ie_guid[j]-2) == 0x1F and

                    // Get current item size and calculate next item position
                    // item_start = @ie_guid[j] - 4
                    // item_size = uint16(item_start) = uint16(@ie_guid[j]-4)
                    // next_item_start = item_start + item_size = @ie_guid[j]-4 + uint16(@ie_guid[j]-4)
                    uint16(@ie_guid[j]-4) > 0 and
                    uint16(@ie_guid[j]-4) <= 512 and
                    @ie_guid[j]-4+uint16(@ie_guid[j]-4) < @lnk_header[i]+0x4E+uint16(@lnk_header[i]+0x4C) and

                    // Next item must have valid size
                    uint16(@ie_guid[j]-4+uint16(@ie_guid[j]-4)) > 0 and
                    uint16(@ie_guid[j]-4+uint16(@ie_guid[j]-4)) <= 512 and
                    @ie_guid[j]-4+uint16(@ie_guid[j]-4)+uint16(@ie_guid[j]-4+uint16(@ie_guid[j]-4)) <= @lnk_header[i]+0x4E+uint16(@lnk_header[i]+0x4C) and

                    // Check if next item contains suspicious content
                    (
                        // Path prefixes: ANCHORED to start of next item data (+3 offset from next_item_start)
                        for any of ($path_unc_a, $path_fwd_a, $path_mix_a, $path_unc3_a, $path_fwd3_a, $path_mix2_a, $path_mix3_a, $path_mix4_a) : (
                            $ at (@ie_guid[j]-4+uint16(@ie_guid[j]-4)+3)
                        )
                        or
                        for any of ($path_unc_w, $path_fwd_w, $path_mix_w, $path_unc3_w, $path_fwd3_w, $path_mix2_w, $path_mix3_w, $path_mix4_w) : (
                            $ at (@ie_guid[j]-4+uint16(@ie_guid[j]-4)+3)
                        )
                        or
                        // Other indicators: anywhere within next item bounds
                        for any of ($davwwwroot, $ssl, $webdav, $file_proto, $port_80, $port_443, $port_8443, $port_ssl, $cdn_pages, $cdn_workers, $cdn_bunny, $exe, $dll, $scr, $bat, $cmd, $ps1, $vbs, $vbe, $js, $jse, $wsf, $wsh, $hta, $lnk, $application, $msi, $msp, $cpl, $jar, $pif, $gadget, $reg) : (
                            $ in (@ie_guid[j]-4+uint16(@ie_guid[j]-4)+2 .. @ie_guid[j]-4+uint16(@ie_guid[j]-4)+uint16(@ie_guid[j]-4+uint16(@ie_guid[j]-4)))
                        )
                    )
                )
            )
        )
}
