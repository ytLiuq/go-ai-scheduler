# Multi-stage build for go-ai-scheduler.
# Default target remains a small runtime image with prebuilt binaries.
# The optional `dev` target keeps the Go toolchain, source tree, and warmed
# module/build caches so `make run-full-stack` can start without first-run
# dependency downloads.

FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p /root/.cache/go-build && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/scheduler ./cmd/scheduler && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/ai-service ./cmd/ai-service && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/migrate ./cmd/migrate

FROM golang:1.23-alpine AS dev
WORKDIR /root/workspace/go-ai-scheduler
RUN apk add --no-cache bash build-base ca-certificates docker-cli make mysql-client redis tzdata
COPY --from=builder /go/pkg/mod /go/pkg/mod
COPY --from=builder /root/.cache/go-build /root/.cache/go-build
COPY . .
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build
CMD ["bash"]

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/ /app/
ENTRYPOINT ["/app/api"]
CMD []
