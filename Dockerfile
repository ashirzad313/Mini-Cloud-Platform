# Build the Go API executable.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api

# Run the executable without including the Go toolchain.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /api /usr/local/bin/mini-cloud-api
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mini-cloud-api"]
