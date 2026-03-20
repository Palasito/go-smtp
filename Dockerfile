# ---- Build Stage ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

# Install CA certificates (needed in final image for HTTPS calls to Microsoft APIs)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy dependency files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
# CGO_ENABLED=0 ensures fully static binary (no libc dependency)
# -ldflags injects version metadata and strips debug info to reduce binary size
# -trimpath removes local filesystem paths from the binary
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build \
    -ldflags="-s -w \
      -X github.com/Palasito/go-smtp/internal/version.Version=${VERSION} \
      -X github.com/Palasito/go-smtp/internal/version.Commit=${COMMIT} \
      -X github.com/Palasito/go-smtp/internal/version.BuildDate=${BUILD_DATE}" \
    -trimpath -o /smtp-relay ./cmd/smtp-relay

# ---- Runtime Stage ----
FROM scratch

# Copy CA certificates for HTTPS (Graph API, Azure login, Key Vault)
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=build /smtp-relay /smtp-relay

# Expose default SMTP and health ports (override at runtime via env vars)
EXPOSE 8025 9090

# Run as non-root user (nobody)
USER 65534:65534

# Run the binary
ENTRYPOINT ["/smtp-relay"]
