# Base image with Java installed
FROM openjdk:11
MAINTAINER val

# Set the working directory inside the container
WORKDIR /app

COPY target/JavaClientSimulator-1.0.jar app.jar
ENTRYPOINT ["sh", "-c", "java -jar app.jar -c \"$JAVA_TEST_CLIENT\" -u \"$JAVA_TARGET_URL\""]
