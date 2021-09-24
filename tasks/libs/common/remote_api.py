import errno
import re

from invoke.exceptions import Exit

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")


class RemoteAPI(object):
    """
    Helper class to perform calls against a given remote API.
    """

    BASE_URL = ""

    def __init__(self):
        self.api_name = "Unknown API"
        self.authorization_error_message = "HTTP 401 Unauthorized"

    def request(
        self,
        path,
        headers=None,
        data=None,
        json_input=False,
        json_output=False,
        stream_output=False,
        raw_output=False,
        method=None,
    ):
        """
        Utility to make a request to a remote API.

        headers: A hash of headers to pass to the request.
        data: An object containing the body of the request.
        json_input: If set to true, data is passed with the json parameter of requests.post instead of the data parameter.

        By default, the request method is GET, or POST if data is not empty.
        method: Can be set to GET, POST, PUT or DELETE to force the REST method used.

        By default, we return the text field of the response object. The following fields can alter this behavior:
        json_output: the json field of the response object is returned.
        stream_output: the request asks for a stream response, and the raw response object is returned.
        raw_output: the content field of the resposne object is returned.
        """
        import requests

        url = self.BASE_URL + path

        # TODO: Use the param argument of requests instead of handling URL params
        # manually
        try:
            # If json_input is true, we specifically want to send data using the json
            # parameter of requests.post / requests.put
            if method == "PUT":
                if json_input:
                    r = requests.put(url, headers=headers, json=data, stream=stream_output)
                else:
                    r = requests.put(url, headers=headers, data=data, stream=stream_output)
            elif method == "DELETE":
                r = requests.delete(url, headers=headers, stream=stream_output)
            elif data or method == "POST":
                if json_input:
                    r = requests.post(url, headers=headers, json=data, stream=stream_output)
                else:
                    r = requests.post(url, headers=headers, data=data, stream=stream_output)
            else:
                r = requests.get(url, headers=headers, stream=stream_output)
            if r.status_code == 401:
                print(self.authorization_error_message)

                print("{} says: {}".format(self.api_name, r.json()["error_description"]))
                raise Exit(code=1)
        except requests.exceptions.Timeout:
            print("Connection to {} ({}) timed out.".format(self.api_name, url))
            raise Exit(code=1)
        except requests.exceptions.RequestException as e:
            m = errno_regex.match(str(e))
            if not m:
                print("Unknown error raised connecting to {} ({}): {}".format(self.api_name, url, e))

            # Parse errno to give a better explanation
            # Requests doesn't have granularity at the level we want:
            # http://docs.python-requests.org/en/master/_modules/requests/exceptions/
            errno_code = int(m.group(1))
            message = m.group(2)

            if errno_code == errno.ENOEXEC:
                exit_msg = "Error resolving {}: {}".format(url, message)
            elif errno_code == errno.ECONNREFUSED:
                exit_msg = "Connection to {} ({}) refused".format(self.api_name, url)
            else:
                exit_msg = "Error while connecting to {}: {}".format(url, str(e))
            raise Exit(message=exit_msg, code=1)

        if json_output:
            return r.json()
        if raw_output:
            return r.content
        if stream_output:
            return r
        return r.text
