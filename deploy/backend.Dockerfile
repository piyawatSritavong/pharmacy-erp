FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download

COPY backend /src/backend
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/pharmacy-erp ./cmd/api

FROM alpine:3.20

# TLS to Supabase goes through the system root store, and a scratch-ish alpine
# has none: without this the pooler connection fails at the handshake with
# "x509: certificate signed by unknown authority", which reads like a database
# problem rather than a missing package.
RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 -h /app pharmacy

WORKDIR /app
COPY --from=builder /out/pharmacy-erp /app/pharmacy-erp

# No uploads directory: product photographs live in Supabase Storage, so the
# container writes nothing to its own filesystem and needs nowhere to write it.
RUN chown -R pharmacy:pharmacy /app

USER pharmacy

# Documentation, not configuration. Render sets PORT in the environment and the
# process listens on whatever it says (config.Load reads PORT, then HTTP_PORT,
# then falls back to 8080) — so this line states the default and nothing binds
# to it if the platform asks for another.
EXPOSE 8080

CMD ["./pharmacy-erp", "serve"]
