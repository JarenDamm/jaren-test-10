# syntax=docker/dockerfile:1
# Owner module: language-go-http
#
# Multi-stage build for the generated Go HTTP service.
#
# Build stage uses golang:<go_version>-alpine, where <go_version>
# matches the toolchain pinned in go.mod (rendered from the same
# .Inputs.go_version input). Runtime is distroless/static:nonroot —
# no shell, no package manager, baked-in nonroot user.
FROM golang:1.24-alpine AS build
WORKDIR /src

# Capability modules contribute `require` lines into go.mod via merge
# regions but do not ship a go.sum (sum tracking happens here at build
# time, not at render time). `go mod tidy` materializes go.sum from the
# rendered go.mod before the build sees it.
COPY . .
RUN go mod tidy

# Static, stripped binary. CGO disabled so the binary runs in
# distroless/static (no libc). The entrypoint lives at ./cmd/server.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/app \
    ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
