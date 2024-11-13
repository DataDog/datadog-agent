# Aferofs lint

This package implements lints for spf13/afero package.

## calls both os and spf13/afero methods

Mixed use may be a problem because changes made in the mock filesystem
will not be present in the real one, and vice versa.

For example:

```go
func TestCleanup(t *testing.T) {
    doodad := newDoodad(afero.NewMemMapFs())
    doodad.frobnicate()
    _, err := os.Stat(doodad.tempFile)
    assert.True(t, os.IsNotExist(err), "temp file should be cleaned up")
```

The component is passed a mock file system that will be used to create
a temp file, but becuase a check is done using the real file system
the test will pass even if the test subject fails to remove the temp
file.

## Limitations

Analysis is limited to a single function at a time.
