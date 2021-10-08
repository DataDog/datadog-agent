FROM datadog/docker-library:python_2_7-alpine3_6

# datadog-py has no release with UDS support yet, using a commit hash
RUN pip install --no-cache-dir https://github.com/DataDog/datadogpy/archive/af1c23bf9cd187208d336f4a1468535f06f43acd.zip
COPY sender.py /sender.py

CMD [ "python", "/sender.py" ]
