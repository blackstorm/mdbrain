FROM node:25-bookworm-slim AS app-builder
WORKDIR /app/server
COPY server/package*.json ./
RUN npm install --include=dev
COPY server/console.css server/app.css ./
COPY server/resources ./resources
RUN npm run build

FROM golang:1.25-bookworm AS backend-builder
WORKDIR /app/server-go
COPY server-go/go.mod server-go/go.sum ./
RUN go mod download
COPY server-go ./
RUN mkdir -p /out && \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mdbrain ./cmd/mdbrain && \
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mdbrain-migrate ./cmd/mdbrain-migrate

# Stage 3: Runtime
FROM debian:bookworm-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    getent group mdbrain >/dev/null || groupadd -r mdbrain; \
    id -u mdbrain >/dev/null 2>&1 || useradd -r -m -g mdbrain -s /bin/sh mdbrain; \
    mkdir -p /app/data; \
    chown -R mdbrain:mdbrain /app

COPY --from=backend-builder --chown=mdbrain:mdbrain /out/mdbrain ./mdbrain
COPY --from=backend-builder --chown=mdbrain:mdbrain /out/mdbrain-migrate ./mdbrain-migrate
COPY --from=backend-builder --chown=mdbrain:mdbrain /app/server-go/ent/migrate/migrations ./server-go/ent/migrate/migrations
COPY --from=app-builder --chown=mdbrain:mdbrain /app/server/resources ./server/resources
COPY docker-entrypoint.sh ./docker-entrypoint.sh
COPY docker-healthcheck.sh ./healthcheck.sh
RUN chmod +x ./mdbrain ./mdbrain-migrate ./docker-entrypoint.sh ./healthcheck.sh

USER mdbrain

ENV APP_PORT=8080
ENV CONSOLE_PORT=9090
ENV DATA_PATH=/app/data
ENV ENVIRONMENT=production

EXPOSE 8080 9090

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/healthcheck.sh"]

CMD ["./docker-entrypoint.sh"]
