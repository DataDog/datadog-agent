ARG MAVEN_IMAGE=669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/maven:3.9.6-eclipse-temurin-8@sha256:c85a28e9be6552408174e726d617a451350576f157aa5b542252efc128055b96
ARG RUNTIME_IMAGE=669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/eclipse-temurin:8-jre-alpine@sha256:55fa30bff6b04913f331f966e6e1ffa0a164523c2002d4a9f9a2a06c77ec3769

FROM ${MAVEN_IMAGE} AS build
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline -q
COPY java ./java
COPY javatest ./javatest
COPY res ./res
RUN mvn clean package -q

FROM ${RUNTIME_IMAGE}
WORKDIR /app

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=build /app/target/azure-files-log-writer.jar /app/app.jar
COPY --from=build /app/target/classes/com/datadoghq/e2e/logwriter/Crc64.class /app/classes/com/datadoghq/e2e/logwriter/Crc64.class
COPY ledger.sh /app/ledger.sh
COPY appender.sh /app/appender.sh
RUN chmod 0555 /app/ledger.sh /app/appender.sh && chown -R appuser:appgroup /app

USER appuser
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
