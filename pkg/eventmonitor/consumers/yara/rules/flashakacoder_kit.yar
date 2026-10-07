rule flashakacoder_kit
{
    meta:
        yarahub_uuid = "24b498ea-b6b3-4794-9e87-6e784803137f"
        yarahub_license = "CC0 1.0"
        yarahub_author_twitter = "@Lenny_3BO"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6d36b6c335935a07afd917574c27f29e"
        author      = "Lenny-3BO"
        description = "FLASHAKACODER PHP banking kit -- operator tag + admin chain + Telegram exfil + HTML form-action"
        family      = "phishkit.flashakacoder"
        version     = "2.1"
        date        = "2026-05-01"
        reference   = "hunts/flashakacoder-tarrarat-cluster"

    strings:
        // PHP source open tag
        $php_open       = "<?php"

        // Operator marker (config.php)
        $op_tag         = "FLASHAKACODER" ascii nocase

        // Kit admin nav chain (filename literals)
        $nav_log        = "admin/log.php" ascii
        $nav_log2       = "admin/log2.php" ascii
        $nav_exp        = "admin/exp.php" ascii
        $nav_views      = "admin/views.php" ascii
        $nav_spin       = "admin/spin.php" ascii

        // HTML form-action variant (kit phishing pages)
        $form_act_log   = "action=\"admin/log.php\"" ascii nocase
        $form_act_log2  = "action=\"admin/log2.php\"" ascii nocase
        $form_act_exp   = "action=\"admin/exp.php\"" ascii nocase
        $form_act_views = "action=\"admin/views.php\"" ascii nocase
        $form_act_spin  = "action=\"admin/spin.php\"" ascii nocase

        // Telegram exfil triplet (admin handlers)
        $tg_api         = "api.telegram" ascii nocase
        $tg_send        = "sendMessage" ascii
        $tg_chat        = "chat_id" ascii nocase

    condition:
        // Not a PE (rules out TarraRAT.Client.exe and other native binaries)
        uint16(0) != 0x5A4D and
        filesize < 2MB and
        (
            // Operator tag in any kit file (config.php)
            $op_tag
            or
            // Two distinct admin-chain refs in PHP source
            ($php_open and 2 of ($nav_log, $nav_log2, $nav_exp, $nav_views, $nav_spin))
            or
            // Telegram exfil triplet in PHP source (admin handlers)
            ($php_open and all of ($tg_api, $tg_send, $tg_chat))
            or
            // HTML form page whose action posts to a kit admin handler
            (1 of ($form_act_log, $form_act_log2, $form_act_exp, $form_act_views, $form_act_spin))
        )
}

// === black_empire_panel.yar ===
/*
 * black_empire_panel.yar
 * Author:  Lenny-3BO
 * Version: 2.0
 * Date:    2026-05-01
 * Family:  TarraRAT C2 server panel (WPF) -- C# source OR compiled PE
 * Description: Matches the TarraRAT C2 operator panel. Targets either the
 *              decompiled / source C# project (Views/*.xaml.cs) or a built
 *              panel binary. Distinguishing markers: BankNotifierWindow
 *              class reference, TarraRAT namespace tag, and WPF/.NET source
 *              shape ("using System" + "namespace"). Excludes the license
 *              server (Node.js, no WPF imports) and TarraRAT.Client.exe
 *              (no BankNotifierWindow class). 178.16.54.149 / /api/validate
 *              act as supporting markers, not required.
 */

