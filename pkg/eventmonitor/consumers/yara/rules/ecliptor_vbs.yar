rule ecliptor_vbs
{
    meta:
        yarahub_uuid = "f18a5f8c-5d46-45a7-9711-b909f09342fc"
        yarahub_license = "CC0 1.0"
        yarahub_author_twitter = "@Lenny_3BO"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6eb2b42aff187c1ed8e517dbba37c3c7"
        author      = "Lenny-3BO"
        description = "Ecliptor VBS dropper -- ChrW arithmetic + Year(Now) sentinel + Timer*Rnd noise"
        family      = "win.ecliptor"
        version     = "2.0"
        date        = "2026-05-01"
        reference   = "hunts/flashakacoder-tarrarat-cluster"

    strings:
        // Required core shape (current variant)
        $chrw           = "ChrW(" ascii wide nocase
        $on_err         = "On Error Resume Next" ascii wide nocase
        $err_clear      = "Err.Clear" ascii wide nocase
        $year_sentinel  = /Year\(Now\) < [12][0-9]{3}/ ascii wide
        $rnd_noise      = /Year\(Now\)\*[0-9]+\+Timer\*Rnd/ ascii wide

        // Legacy/optional markers (older variants)
        $btdf_call      = /bt\.df\(\d{4,6}\)/ ascii wide
        $aes_cbc        = "AES-CBC" ascii wide nocase
        $persist_marker = "ECLIPTOR_SYS_INIT" ascii wide nocase
        $func_name1     = "MsqBIbY" ascii wide
        $func_name2     = "hBFCu" ascii wide

    condition:
        // UTF-16LE BOM or first-byte heuristic for VBS source
        filesize < 5MB and
        $chrw and
        (
            // Current variant: dead-code year sentinel + Timer*Rnd noise
            ($year_sentinel and $rnd_noise and $on_err and $err_clear)
            or
            // Legacy variant
            ($btdf_call and ($aes_cbc or $persist_marker or 1 of ($func_name1, $func_name2)))
        )
}

// === flashakacoder_kit.yar ===
/*
 * flashakacoder_kit.yar
 * Author:  Lenny-3BO
 * Version: 2.1
 * Date:    2026-05-01
 * Family:  FLASHAKACODER PHP banking phishing kit
 * Description: Matches the FLASHAKACODER operator phishing kit (BankUnited
 *              tenant + sibling kits). Targets per-file:
 *              (a) PHP source modules with FLASHAKACODER tag, kit nav chain,
 *                  or Telegram exfil triplet
 *              (b) HTML form pages whose <form action="..."> points at the
 *                  kit's admin handlers (admin/log.php / admin/log2.php /
 *                  admin/exp.php / admin/views.php / admin/spin.php).
 *              Excluded confounders: TarraRAT.Client.exe (PE),
 *              MainWindow.xaml.cs (C# source), license server.js (Node),
 *              decoded VBS (Ecliptor loader).
 */

