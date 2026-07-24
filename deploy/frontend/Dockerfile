FROM node:22-alpine AS build
WORKDIR /src
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=visionai-pnpm-store,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store \
 && pnpm config set network-concurrency 16 \
 && pnpm config set fetch-retries 5 \
 && pnpm config set fetch-retry-maxtimeout 120000 \
 && pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build:prod

FROM nginx:1.29.5-alpine
COPY deploy/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist-prod /usr/share/nginx/html
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=20 \
  CMD wget -q -O - http://127.0.0.1/healthz || exit 1
