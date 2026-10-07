rule tarrarat_assembly
{
    meta:
        yarahub_uuid = "144f59a0-d984-4bf8-b038-9bc181b1a7a5"
        yarahub_license = "CC0 1.0"
        yarahub_author_twitter = "@Lenny_3BO"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a44fd5e62ca073c5b45d782cdb4624ee"
        author      = "Lenny-3BO"
        description = "TarraRAT (RSquare) .NET RAT client -- assembly + config indicators"
        family      = "win.tarrarat"
        version     = "1.0"
        date        = "2026-04-30"
        reference   = "hunts/omegatech-as202412-tun1-sweep"

    strings:
        // Default mutex prefix constructed as RSQ_ + SHA256(MachineName)
        $mutex_prefix   = "RSQ_" ascii wide

        // Default C2 port 4782 as UTF-16 or decimal string in settings
        $port_str       = "4782" ascii wide

        // AES-GCM + SHA256 key derivation class name
        $aes_class      = "Aes256" ascii wide

        // MessagePack serializer reference
        $msgpack        = "MessageSerializer" ascii wide

        // TarraRAT namespace root
        $ns_root        = "TarraRAT." ascii wide

    condition:
        uint16(0) == 0x5A4D and
        filesize < 20MB and
        $mutex_prefix and
        2 of ($port_str, $aes_class, $msgpack, $ns_root)
}

// === needle_panel.yar ===
/*
 * needle_panel.yar
 * Author:  Lenny-3BO
 * Version: 1.0
 * Date:    2026-04-30
 * Family:  TarraRAT Node.js license server
 * Description: Matches the TarraRAT Node.js Telegram-bot license server (server.js)
 *              via Express route /api/validate on PORT 3000, node-telegram-bot-api,
 *              HWID-binding workflow comments, and licenses.db schema strings.
 *              Deployed on 178.16.54.149:3000.
 */

