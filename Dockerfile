# --- Dashboard build ---
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go build ---
FROM golang:1.26.5-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /lighthouse ./cmd/lighthouse \
    && mkdir /data

# --- Runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /lighthouse /lighthouse
# Writable record book directory for the nonroot (65532) runtime user.
COPY --from=build --chown=65532:65532 /data /data

ENV LIGHTHOUSE_RECORDBOOK_PATH=/data/recordbook
VOLUME /data

# DNS, dashboard/API, ops (metrics + probes)
EXPOSE 53/udp 53/tcp 853/tcp 443/tcp 8080/tcp 9090/tcp

ENTRYPOINT ["/lighthouse"]
