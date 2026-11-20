FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/parachute ./cmd/parachute

FROM alpine:3.22@sha256:d406446bc05b82480756ce8f1287964d28768c23955fa9480d2a74a2eae7a654
RUN mkdir -p /var/lib/parachute
COPY --from=build /out/parachute /usr/local/bin/parachute
LABEL parachute.self=true
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --start-interval=1s CMD ["parachute", "healthcheck"]
ENTRYPOINT ["parachute"]
