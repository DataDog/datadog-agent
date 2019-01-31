
set GLIB-URL="http://ftp.gnome.org/pub/gnome/binaries/win64/glib/2.26/glib_2.26.1-1_win64.zip"
set PKG-CONFIG-URL="http://ftp.gnome.org/pub/gnome/binaries/win64/dependencies/pkg-config_0.23-2_win64.zip"
set GETTEXT-URL="http://ftp.gnome.org/pub/gnome/binaries/win64/dependencies/gettext-runtime_0.18.1.1-2_win64.zip"
mkdir c:\deps
pushd c:\deps
curl -fsS -o glib.zip %GLIB-URL%
curl -fsS -o pkg-config.zip %PKG-CONFIG-URL%
curl -fsS -o gettext.zip %GETTEXT-URL%
7z x *.zip > NUL
go version
popd
