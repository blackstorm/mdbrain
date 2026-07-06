FROM oven/bun:1.3.14 AS builder
WORKDIR /app

COPY package.json bun.lock bunfig.toml tsconfig.base.json ./
COPY server/package.json server/package.json
COPY apps/web/package.json apps/web/package.json
COPY packages/shared/package.json packages/shared/package.json
COPY packages/ui/package.json packages/ui/package.json
COPY obsidian-plugin/package.json obsidian-plugin/package.json

RUN bun install --frozen-lockfile

COPY server ./server
COPY apps ./apps
COPY packages ./packages

RUN bun run --cwd server build
RUN bun run --cwd apps/web build

FROM oven/bun:1.3.14
WORKDIR /app

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
  getent group mdbrain >/dev/null || groupadd -r mdbrain; \
  id -u mdbrain >/dev/null 2>&1 || useradd -r -m -g mdbrain -s /bin/sh mdbrain; \
  mkdir -p /app/data; \
  chown -R mdbrain:mdbrain /app

COPY --from=builder --chown=mdbrain:mdbrain /app/apps/web/dist ./apps/web/dist
COPY --from=builder --chown=mdbrain:mdbrain /app/server/resources ./server/resources
COPY docker-entrypoint.sh ./docker-entrypoint.sh
COPY docker-healthcheck.sh ./healthcheck.sh
RUN chmod +x ./docker-entrypoint.sh ./healthcheck.sh

USER mdbrain

ENV HOST=0.0.0.0
ENV APP_PORT=8080
ENV CONSOLE_PORT=9090
ENV DATA_PATH=/app/data
ENV ENVIRONMENT=production

EXPOSE 8080 9090

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/healthcheck.sh"]

CMD ["./docker-entrypoint.sh"]
