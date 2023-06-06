JavaClientSimulator allows testing different java **https** frameworks.
You can run the attached docker or directly by executing the jar package.1

This test util was built using JDK 11

## Configuration

you can control
- java client framework to use. Currently supported: ApacheHttp, OkHttp, URLConnection, HttpClient
- target url
- iterations - number of requests to send. By default infinitely
- timeout - interval between each iteration. By default 1 second

## Standalone:

Build: `mvn clean package`

Run:  `java -jar ./target/JavaClientSimulator-1.0.jar client=<client> url=<url>`

## Docker

Build: `docker build -t java-http-client .`

Run: `docker run -e JAVA_TEST_CLIENT=<client> -e JAVA_TARGET_URL=<url> java-http-client`

