@set tool=%cd%\{{tool}}
@cd /D %{{env_var}}%
@%tool% %*
