# Build stage (needs Go >=1.23)
FROM golang:1.23-bullseye AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Runtime stage (still minimal)
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/main .

EXPOSE 8080
CMD ["./main"]
