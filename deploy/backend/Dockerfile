FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/visionai-server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/visionai-orchestrator ./cmd/orchestrator

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata docker-cli
WORKDIR /app
COPY --from=build /out/visionai-server /out/visionai-orchestrator /app/
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=30 \
  CMD wget -q -O - http://127.0.0.1:58080/health || exit 1
ENTRYPOINT ["/app/visionai-server"]
