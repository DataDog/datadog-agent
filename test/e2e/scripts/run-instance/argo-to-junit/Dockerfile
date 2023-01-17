FROM gcr.io/google-containers/python:3.5.1-alpine

COPY requirements.txt argo_to_junit.py entrypoint.sh /
RUN pip3 install -r requirements.txt

ENTRYPOINT ["/entrypoint.sh"]
