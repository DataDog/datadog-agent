rule Detect_TSUNAMI {
    meta:
        description = "Detects malicious Python scripts carrying the TSUNAMI suite"
        author = "Sn0wFr0$t"
        date = "2024-11-16"
        reference = "Custom rule for detecting TSUNAMI suite malicious scripts"
        severity = "high"
		date = "2024-11-16"
		yarahub_uuid = "638a6ab2-1ecc-4667-91b6-05ce63aefb99"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "3d552bcfef37584f506caa736ca04af5"
    
    strings:
        $tsi1 = "TSUNAMI_PAYLOAD_NAME" nocase
        $tsi2 = "TSUNAMI_PAYLOAD_FOLDER" nocase
        $tsi3_1 = "TSUNAMI_PAYLOAD_PATH" nocase
        $tsi3_2 = "rf\"{TSUNAMI_PAYLOAD_FOLDER}\\{TSUNAMI_PAYLOAD_NAME}\"" nocase
        $tsi4 = "TSUNAMI_PAYLOAD_SCRIPT" nocase

        $tsb1 = "TSUNAMI_INJECTOR_FOLDER" nocase
        $tsb2 = "TSUNAMI_INJECTOR_NAME" nocase
        $tsb3 = "{APPDATA_ROAMING_DIRECTORY}/Microsoft/Windows/Start Menu/Programs/Startup" nocase
        $tsb4 = "Windows Update Script.pyw" nocase

        $tsm1 = "http://{host1}:1224" nocase
        $tsm2 = "subprocess.Popen([sys.executable, ap], creationflags=subprocess.CREATE_NO_WINDOW | subprocess.CREATE_NEW_PROCESS_GROUP)" nocase
        

        $tspy1 = "findstr /v /i \"node_modules .css .svg readme license robots vendor Pods .git .github .node-gyp .nvm debug .local .cache .pyp .pyenv next.config .qt .dex __pycache__ tsconfig.json tailwind.config svelte.config vite.config webpack.config postcss.config prettier.config angular-config.json yarn .gradle .idea .htm .html .cpp .h .xml .java .lock .bin .dll .pyi\""
        $tspy2 = "'find . -type d -name \"node_modules .css .svg readme license robots vendor Pods .git .github .node-gyp .nvm debug .local .cache .pyp .pyenv next.config .qt .dex __pycache__ tsconfig.json tailwind.config svelte.config vite.config webpack.config postcss.config prettier.config angular-config.json yarn .gradle .idea .htm .html .cpp .h .xml .java .lock .bin .dll .pyi\" -prune -o -name ' + pat + ' -print'"
        $tspy3 = "1:A.ssh_obj,2:A.ssh_cmd,3:A.ssh_clip,4:A.ssh_run,5:A.ssh_upload,6:A.ssh_kill,7:A.ssh_any,8:A.ssh_env"
        $tspy4 = "A.cmds:tg=A.cmds[c];t=Thread(target=tg,args=(args,));t.start()#tg(args)"
        $tspy5 = "cmd:sdir=cmd['sdir'];dn=cmd['dname'];sdir=sdir.strip();dn=dn.strip();A.ss_upd(D,cmd,sdir,dn);return _T"
        $tspy6 = "hm = pyHook.HookManager();hm.MouseLeftDown = hmld;hm.MouseRightDown = hmrd;hm.KeyDown = hkb;hm.HookMouse();hm.HookKeyboard()"
        $tspy7 = "\n**\n-[ {text} | PID: {pid}-{c_win}\n-[ @ {t_s} | {event.WindowName}\n**\n"
        $tspy8 = "Chrome & Browser are terminated"

        $tsp1 = "TSUNAMI_INSTALLER_NAME" nocase
        $tsp2_1 = "TSUNAMI_INSTALLER_FOLDER" nocase
        $tsp2_2 = "rf\"{ROAMING_APPDATA_PATH}\\Microsoft\\Windows\\Applications\"" nocase
        $tsp3_1 = "TSUNAMI_INSTALLER_PATH" nocase
        $tsp3_2 = "rf\"{TSUNAMI_INSTALLER_FOLDER}\\Runtime Broker.exe\"" nocase
        $tsp4 = "New-ScheduledTaskAction -Execute \"{TSUNAMI_INSTALLER_PATH}\"" nocase
        $tsp5 = "$Settings -TaskName \"Runtime Broker\"" nocase
        $tsp6 = "rf\"{ROAMING_APPDATA_PATH}\\Microsoft\\Windows\\Applications\\Runtime Broker.exe\"" nocase
        $tsp7 = "rf\"{LOCAL_APPDATA_PATH}\\Microsoft\\Windows\\Applications\\Runtime Broker.exe\"" nocase
        $tsp8 = "rf\"{LOCAL_APPDATA_PATH}\\Microsoft\\Windows\\Applications\\msedge.exe\"" nocase
	$tsp9 = "Runtime Broker" nocase
    
    condition:
        (2 of ($tsi1, $tsi2, $tsi3_1,$tsi3_2,$tsi4)) or
        (2 of ($tsb1, $tsb2,$tsb3,$tsb4)) or
	(2 of ($tsm1,$tsm2)) or
	(2 of ($tspy1,$tspy2,$tspy3,$tspy4,$tspy5,$tspy6,$tspy1,$tspy7,$tspy8)) or
        (3 of ($tsp1,$tsp2_1,$tsp2_2,$tsp3_1,$tsp3_2,$tsp4,$tsp5,$tsp6,$tsp7,$tsp8,$tsp9))
}