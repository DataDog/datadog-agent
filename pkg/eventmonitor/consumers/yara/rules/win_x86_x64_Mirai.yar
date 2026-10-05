rule win_x86_x64_Mirai {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2025-01-26"
        description = "Detects Mirai"
        yarahub_uuid = "c0257af9-5608-4c28-8a94-4060ddd22602"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "800dcb9f93715f5ed7189be2e35aebd9"
        malpedia_family = "win.mirai"
        
    strings:
        $c2_send = {
            4? 8b ??       // MOV   RAX,qword ptr [->WS2_32.DLL::send]
            ?? ?? ?? ??
            ff d?          // CALL  RAX=>WS2_32.DLL::send
            4? 8d ??       // LEA   RAX,[s_win_x86_x64_14000c351] = "win_x86_x64"
            ?? ?? ?? ??
            48 89 ??       // MOV   qword ptr [RBP + local_18],RAX=>s_win_x86_x64_ = "win_x86_x64"
            ?? ?? ?? ??
            4? 8b ??       // MOV   RAX,qword ptr [RBP + local_18]
            ?? ?? ?? ??
            4? 89 ??       // MOV   _Argc=>s_win_x86_x64_14000c351,RAX = "win_x86_x64"
            e8 ?? ??       // CALL  strlen
            ?? ??
        }

        $c2_recv = {
            4? 8b ??       // MOV   RAX,qword ptr [->WS2_32.DLL::recv]
            ?? ?? ?? ??
            ff d?          // CALL  RAX=>WS2_32.DLL::recv
            89 ?? ??       // MOV   dword ptr [RBP + local_24],EAX
            ?? ?? ??
            83 b? ??       // CMP   dword ptr [RBP + local_24],0x0
            ?? ?? ?? ??
            79 ??          // JNS   LAB_140003ef5
            4? 8d ??       // LEA   _Argc,[s_recv_failed_14000c35d] = "recv failed"
            ?? ?? ?? ??
            e8 ?? ??       // CALL  perror
            ?? ??
            e9 ?? ??       // JMP   LAB_140003f75
            ?? ??
        }

    condition:
        all of them
}