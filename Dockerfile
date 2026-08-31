# syntax=docker/dockerfile:1

FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X github.com/rlnorthcutt/reflector/internal/identity.Version=${VERSION}" \
    -o /out/reflector ./cmd/reflector

# Scratch: no shell, no package manager, just the static binary. Reflector
# is a server, not an outbound TLS client, so no CA certificate bundle is
# needed either.
FROM scratch

COPY --from=build /out/reflector /reflector

WORKDIR /app

# Traffic port, then admin API port; see PLAN.md for the full flag set.
EXPOSE 8080 8081

# Mount routes.d/ and payloads/ here to add routes without rebuilding the
# image, e.g.:
#   docker run -v $(pwd)/routes.d:/app/routes.d -v $(pwd)/payloads:/app/payloads ...
VOLUME ["/app/routes.d", "/app/payloads"]

# Numeric UID: scratch has no /etc/passwd, but Docker doesn't need one to
# run as a non-root user.
USER 65532:65532

ENTRYPOINT ["/reflector"]
