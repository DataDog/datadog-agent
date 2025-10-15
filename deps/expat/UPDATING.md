# How to update expat

## workflow using configure2bazel

- download the new release to /tmp/expat-2.5.0

```
C2B=configure2bazel
PLATFORM=macos  # or linux
mkdir /tmp/$PLATFORM
cd /tmp/$PLATFORM
/tmp/expat-2.5.0/configure --disable-static --without-examples --without-tests
make
cd ..

python3 analyze.py /tmp/expat-2.5.0 /tmp/$PLATFORM $PLATFORM overlay
python3 copy_out.py /tmp/expat-2.5.0 /tmp/$PLATFORM $PLATFORM overlay

## About expat

- Not CPU specific, only OS
- Only one file configured `expat_config.h`.  They had the decency to name it expat_config.h and not just config.h

## Future

- Automate generating the files section for the http_archive rule.  Can we make it a module extenstion that could do the glob.
