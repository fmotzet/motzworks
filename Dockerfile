# syntax=docker/dockerfile:1

# --- Stage 1: build the React/TS dashboard ---
FROM node:24-alpine AS ui
WORKDIR /ui
COPY web/ui/package.json web/ui/package-lock.json ./
RUN npm ci
COPY web/ui/ ./
# Redirect the build output to /out (the vite config's default outDir points
# into the Go tree, which doesn't exist in this isolated stage).
RUN npm run build -- --outDir /out --emptyOutDir

# --- Stage 2: build the static Go binary (embeds the dashboard) ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Overwrite the committed dashboard with the freshly built one.
COPY --from=ui /out ./internal/web/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/stock3/motzworks/internal/version.Version=${VERSION}" \
    -o /motzworks ./cmd/motzworks

# --- Stage 3: runtime ---
# Python + impacket are required by the WMI (Windows/DCOM) collector, so the
# runtime is a slim Python base rather than distroless-static.
FROM python:3.12-slim AS runtime
RUN pip install --no-cache-dir impacket \
    && useradd -r -u 65532 motzworks
COPY --from=build /motzworks /usr/local/bin/motzworks
USER motzworks
EXPOSE 8080
ENTRYPOINT ["motzworks"]
CMD ["serve", "-migrate"]
