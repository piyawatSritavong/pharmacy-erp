FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download

COPY backend /src/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/pharmacy-erp ./cmd/api

FROM alpine:3.20

WORKDIR /app
COPY --from=builder /out/pharmacy-erp /app/pharmacy-erp

EXPOSE 8080

CMD ["./pharmacy-erp", "serve"]
