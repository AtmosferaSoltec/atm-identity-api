# syntax=docker/dockerfile:1

# Production image. Built in GitHub Actions and published to GHCR: the server
# never compiles, it only pulls.

FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/identity-api ./cmd/server

FROM alpine:3.22 AS runner
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/identity-api /app/identity-api
USER app
ENV PORT=8080 APP_ENV=production
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/app/identity-api", "healthcheck"]
ENTRYPOINT ["/app/identity-api"]
