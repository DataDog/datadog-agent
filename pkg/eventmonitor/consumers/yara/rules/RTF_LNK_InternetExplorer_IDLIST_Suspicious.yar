/*
    YARA Rule for detecting RTF files with hex-encoded Shell.Explorer.1 LNK IDLIST

    RTF \objdata hex-encodes OLE objects as continuous ASCII hex.
    All offsets relative to IE GUID match (32 hex chars):
      +36: URI item type + flags + zeros + indicator data
           (32 GUID + 4 size field = 36)
      +48: URI string data start (36 + 12 for 6-byte header)

    Each anchored indicator pattern includes the 6-byte URI item header
    (type=0x61, flags, data_size=0, unknown=0) fused with the
    indicator string for better YARA atom extraction.

    Loose indicators ($ind_*) search within 8192 bytes of URI data
    for suspicious domains, WebDAV markers, and executable extensions.
    All patterns are fully case-insensitive on both the original binary
    value AND hex digit case (a-f/A-F) via byte alternatives.

    Structure of type 0x61 URI item (per libfwsi / Joachim Metz):
      [2 size][1 type=0x61][1 flags][2 data_size][2 unknown][var URI]
      flags & 0x80 = UTF-16LE, else ASCII

    Assumes IE root folder is first IDLIST entry (fixed 0x14 bytes).
    LNK header validated near ie_guid - 164 (with up to 128 bytes
    tolerance for RTF whitespace interspersed in the hex stream).
*/

rule RTF_LNK_InternetExplorer_IDLIST_Suspicious
{
    meta:
        author = "node5"
        description = "Detects RTF with hex-encoded OLE LNK containing IE IDLIST with suspicious URI items"
        date = "2026-02-09"
        yarahub_author_twitter = "@node5"
        yarahub_reference_md5 = "7c396677848776f9824ebe408bbba943"
        yarahub_uuid = "8b7214d4-d9b4-449b-ba8c-58ab51c34097"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $rtf_magic = "{\\rtf" ascii

        // Hex-encoded LNK header (20 bytes, 40 hex chars)
        $lnk_hex = "4c0000000114020000000000c000000000000046" ascii nocase

        // Hex-encoded IE GUID (16 bytes, 32 hex chars)
        $ie_guid_hex = "80531c87a0426910a2ea08002b30309d" ascii nocase

        // ================================================================
        // ANCHORED patterns: fused URI item header + indicator at +36
        // ================================================================

        // ---- Unicode indicators (flags=0x80) ----
        // Prefix: 61 80 00 00 00 00 -> hex chars "618000000000"

        $file_w = {
            36 31 38 30 30 30 30 30 30 30 30 30 // 61 80 0000 0000
            (36|34) 36 30 30                     // f/F 00
            (36|34) 39 30 30                     // i/I 00
            (36|34) (63|43) 30 30                // l/L 00
            (36|34) 35 30 30                     // e/E 00
            33 (61|41) 30 30                     // :   00
        }

        $path_unc_w = {
            36 31 38 30 30 30 30 30 30 30 30 30 // 61 80 0000 0000
            35 (63|43) 30 30                     // \ 00
            35 (63|43) 30 30                     // \ 00
            ?? ?? 30 30                          // ? 00
        }

        $path_fwd_w = {
            36 31 38 30 30 30 30 30 30 30 30 30 // 61 80 0000 0000
            32 (66|46) 30 30                     // / 00
            32 (66|46) 30 30                     // / 00
            ?? ?? 30 30                          // ? 00
        }

        $path_mix_w = {
            36 31 38 30 30 30 30 30 30 30 30 30 // 61 80 0000 0000
            35 (63|43) 30 30                     // \ 00
            32 (66|46) 30 30                     // / 00
            ?? ?? 30 30                          // ? 00
        }

        // ---- ASCII indicators (flags=0x00) ----
        // Prefix: 61 00 00 00 00 00 -> hex chars "610000000000"

        $file_a = {
            36 31 30 30 30 30 30 30 30 30 30 30 // 61 00 0000 0000
            (36|34) 36                           // f/F
            (36|34) 39                           // i/I
            (36|34) (63|43)                      // l/L
            (36|34) 35                           // e/E
            33 (61|41)                           // :
        }

        $path_unc_a = {
            36 31 30 30 30 30 30 30 30 30 30 30 // 61 00 0000 0000
            35 (63|43) 35 (63|43) ?? ??          // \\ + byte
        }

        $path_fwd_a = {
            36 31 30 30 30 30 30 30 30 30 30 30 // 61 00 0000 0000
            32 (66|46) 32 (66|46) ?? ??          // // + byte
        }

        $path_mix_a = {
            36 31 30 30 30 30 30 30 30 30 30 30 // 61 00 0000 0000
            35 (63|43) 32 (66|46) ?? ??          // \/ + byte
        }

        // ================================================================
        // LOOSE indicators: UTF-16LE hex-encoded, within URI data window
        // All case-insensitive via byte alternatives for both original
        // binary char case AND hex digit case (a-f/A-F)
        // ================================================================

        // ---- Suspicious domains ----

        $ind_pages_dev_w = {
            (35|37) 30 30 30                   // p/P 00
            (34|36) 31 30 30                   // a/A 00
            (34|36) 37 30 30                   // g/G 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 33 30 30                   // s/S 00
            32 (45|65) 30 30                   // . 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 36 30 30                   // v/V 00
        }

        $ind_workers_dev_w = {
            (35|37) 37 30 30                   // w/W 00
            (34|36) (46|66) 30 30              // o/O 00
            (35|37) 32 30 30                   // r/R 00
            (34|36) (42|62) 30 30              // k/K 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 32 30 30                   // r/R 00
            (35|37) 33 30 30                   // s/S 00
            32 (45|65) 30 30                   // . 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 36 30 30                   // v/V 00
        }

        $ind_b_cdn_net_w = {
            (34|36) 32 30 30                   // b/B 00
            32 (44|64) 30 30                   // - 00
            (34|36) 33 30 30                   // c/C 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) (45|65) 30 30              // n/N 00
            32 (45|65) 30 30                   // . 00
            (34|36) (45|65) 30 30              // n/N 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 34 30 30                   // t/T 00
        }

        // ---- WebDAV / protocol indicators ----

        $ind_webdav_w = {
            (35|37) 37 30 30                   // w/W 00
            (34|36) 35 30 30                   // e/E 00
            (34|36) 32 30 30                   // b/B 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 36 30 30                   // v/V 00
        }

        $ind_ssl_w = {
            34 30 30 30                        // @ 00
            (35|37) 33 30 30                   // S/s 00
            (35|37) 33 30 30                   // S/s 00
            (34|36) (43|63) 30 30              // L/l 00
        }

        $ind_davwwwroot_w = {
            (34|36) 34 30 30                   // D/d 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 36 30 30                   // v/V 00
            (35|37) 37 30 30                   // W/w 00
            (35|37) 37 30 30                   // W/w 00
            (35|37) 37 30 30                   // W/w 00
            (35|37) 32 30 30                   // R/r 00
            (34|36) (46|66) 30 30              // o/O 00
            (34|36) (46|66) 30 30              // o/O 00
            (35|37) 34 30 30                   // t/T 00
        }

        // ---- Executable extensions (case-insensitive UTF-16LE hex) ----

        $ind_exe_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 38 30 30                   // x/X 00
            (34|36) 35 30 30                   // e/E 00
        }

        $ind_dll_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) (43|63) 30 30              // l/L 00
            (34|36) (43|63) 30 30              // l/L 00
        }

        $ind_scr_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 33 30 30                   // s/S 00
            (34|36) 33 30 30                   // c/C 00
            (35|37) 32 30 30                   // r/R 00
        }

        $ind_bat_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 32 30 30                   // b/B 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 34 30 30                   // t/T 00
        }

        $ind_cmd_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 33 30 30                   // c/C 00
            (34|36) (44|64) 30 30              // m/M 00
            (34|36) 34 30 30                   // d/D 00
        }

        $ind_ps1_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 30 30 30                   // p/P 00
            (35|37) 33 30 30                   // s/S 00
            33 31 30 30                        // 1 00
        }

        $ind_vbs_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 36 30 30                   // v/V 00
            (34|36) 32 30 30                   // b/B 00
            (35|37) 33 30 30                   // s/S 00
        }

        $ind_vbe_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 36 30 30                   // v/V 00
            (34|36) 32 30 30                   // b/B 00
            (34|36) 35 30 30                   // e/E 00
        }

        $ind_js_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (41|61) 30 30              // j/J 00
            (35|37) 33 30 30                   // s/S 00
        }

        $ind_jse_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (41|61) 30 30              // j/J 00
            (35|37) 33 30 30                   // s/S 00
            (34|36) 35 30 30                   // e/E 00
        }

        $ind_wsf_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 37 30 30                   // w/W 00
            (35|37) 33 30 30                   // s/S 00
            (34|36) 36 30 30                   // f/F 00
        }

        $ind_wsh_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 37 30 30                   // w/W 00
            (35|37) 33 30 30                   // s/S 00
            (34|36) 38 30 30                   // h/H 00
        }

        $ind_hta_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 38 30 30                   // h/H 00
            (35|37) 34 30 30                   // t/T 00
            (34|36) 31 30 30                   // a/A 00
        }

        $ind_lnk_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (43|63) 30 30              // l/L 00
            (34|36) (45|65) 30 30              // n/N 00
            (34|36) (42|62) 30 30              // k/K 00
        }

        $ind_application_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 30 30 30                   // p/P 00
            (35|37) 30 30 30                   // p/P 00
            (34|36) (43|63) 30 30              // l/L 00
            (34|36) 39 30 30                   // i/I 00
            (34|36) 33 30 30                   // c/C 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 34 30 30                   // t/T 00
            (34|36) 39 30 30                   // i/I 00
            (34|36) (46|66) 30 30              // o/O 00
            (34|36) (45|65) 30 30              // n/N 00
        }

        $ind_msi_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (44|64) 30 30              // m/M 00
            (35|37) 33 30 30                   // s/S 00
            (34|36) 39 30 30                   // i/I 00
        }

        $ind_msp_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (44|64) 30 30              // m/M 00
            (35|37) 33 30 30                   // s/S 00
            (35|37) 30 30 30                   // p/P 00
        }

        $ind_cpl_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 33 30 30                   // c/C 00
            (35|37) 30 30 30                   // p/P 00
            (34|36) (43|63) 30 30              // l/L 00
        }

        $ind_jar_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) (41|61) 30 30              // j/J 00
            (34|36) 31 30 30                   // a/A 00
            (35|37) 32 30 30                   // r/R 00
        }

        $ind_pif_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 30 30 30                   // p/P 00
            (34|36) 39 30 30                   // i/I 00
            (34|36) 36 30 30                   // f/F 00
        }

        $ind_gadget_w = {
            32 (45|65) 30 30                   // . 00
            (34|36) 37 30 30                   // g/G 00
            (34|36) 31 30 30                   // a/A 00
            (34|36) 34 30 30                   // d/D 00
            (34|36) 37 30 30                   // g/G 00
            (34|36) 35 30 30                   // e/E 00
            (35|37) 34 30 30                   // t/T 00
        }

        $ind_reg_w = {
            32 (45|65) 30 30                   // . 00
            (35|37) 32 30 30                   // r/R 00
            (34|36) 35 30 30                   // e/E 00
            (34|36) 37 30 30                   // g/G 00
        }

    condition:
        $rtf_magic at 0 and
        filesize < 50MB and
        filesize > 100 and

        for any j in (1..#ie_guid_hex) : (
            // LNK header ~164 hex chars before IE GUID; allow up to 128 bytes
            // of RTF whitespace (CRLF, spaces) interspersed in the hex stream
            for any of ($lnk_hex) : (
                $ in (@ie_guid_hex[j] - 292 .. @ie_guid_hex[j] - 164)
            ) and
            (
                // Strict: file:/UNC anchored at exact URI item start
                for any of ($file_*, $path_*) : (
                    $ at (@ie_guid_hex[j] + 36)
                )
                or
                // Loose: suspicious indicators within URI item data window
                for any of ($ind_*) : (
                    $ in (@ie_guid_hex[j] + 48 .. @ie_guid_hex[j] + 48 + 8192)
                )
            )
        )
}
