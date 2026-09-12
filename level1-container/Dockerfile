# --- build stage ---
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# --- run stage ---
FROM alpine:3.19
RUN addgroup -S app && adduser -S app -G app \
    && mkdir -p /var/log && chown app:app /var/log
WORKDIR /app
COPY --from=builder /app/server ./server
USER app

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["./server"]
