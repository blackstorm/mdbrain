#!/usr/bin/env sh
set -eu

exec java -Djava.awt.headless=true --enable-native-access=ALL-UNNAMED ${JAVA_OPTS:-} -jar /app/app.jar
