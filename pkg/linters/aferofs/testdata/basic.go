package basic

import (
	"os"

	"github.com/spf13/afero"
)

func withMethods1(f *os.File, fs afero.Fs) {
	f.Write([]byte{}) // want `calls both os and spf13/afero methods`
	fs.Stat("/")
}

func withMethods2(f *os.File, fs afero.Fs) {
	fs.Stat("/")
	f.Write([]byte{}) // want `calls both os and spf13/afero methods`
}

func mixed1(fs afero.Fs) {
	fs.Create("/foo")
	os.Stat("/bar") // want `calls both os and spf13/afero methods`
}

func package1() {
	fs := afero.NewMemMapFs()
	os.Stat("/") // want `calls both os and spf13/afero methods`
	_ = fs
}

func maybeFine(fs afero.Fs) {
	os.Stat("/bar")
}
