# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/pc-control ./cmd/pc-control

FROM scratch

COPY --from=build /out/pc-control /pc-control

# The service uses only unprivileged TCP and UDP sockets.
USER 65532:65532

ENTRYPOINT ["/pc-control"]
