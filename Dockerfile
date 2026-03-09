# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY main.go ./
COPY web ./web

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/costweb .

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=build /out/costweb /app/costweb
COPY web /app/web

EXPOSE 8080

ENV PORT=8080

CMD ["/app/costweb"]
