RIDK installation specifics

During ami rebuilds for the last years, it was noted, that ruby
environment rapidly changes for older ruby versions, where 2.4 family is 
no longer supported.

On a current round `ridk install 1` , `ridk install 2` are skipped,  while
needed ruby libraries are provided by MSYS package as of 2019 pinned to version 
and installed by choco.

In normal circumstances proper sequence is `ridk install 1 2 3` without any chores.  

```sh
ridk install 1
bash -c "yes | pacman -R catgets libcatgets"
ridk install 2
ridk install 3
```

Improval points possibly

mklink /J %GOPATH%\src\github.com\StackVista\stackstate-agent %WIN_CI_PROJECT%

