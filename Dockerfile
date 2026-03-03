# syntax = docker/dockerfile:1.2

# get modules, if they don't change the cache can be used for faster builds
FROM --platform=$BUILDPLATFORM golang:1.20 AS base
ENV GO111MODULE=on
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# build the application
FROM base AS build
ARG TARGETOS
ARG TARGETARCH
# temp mount all files instead of loading into image with COPY
# temp mount module cache
# temp mount go build cache
COPY . .
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-w -s" -o /app/build/app ./cmd/go-template/*.go

# Import the binary from build stage
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12 as prd
COPY --from=build /app/build/app /app
# this is the numeric version of user nonroot:nonroot to check runAsNonRoot in kubernetes
USER 65532:65532
ENTRYPOINT ["/app"]
