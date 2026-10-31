FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
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
