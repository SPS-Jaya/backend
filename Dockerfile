# Use the official Golang image to create a build artifact
FROM golang:1.21-alpine as builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Use a minimal alpine image
FROM alpine:latest

# Install required packages and Cloud SQL proxy
RUN apk --no-cache add \
    ca-certificates \
    wget \
    && wget https://dl.google.com/cloudsql/cloud_sql_proxy.linux.amd64 -O /cloud_sql_proxy \
    && chmod +x /cloud_sql_proxy \
    && mkdir -p /cloudsql \
    && chmod 777 /cloudsql

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/main .

# Expose port 8080
EXPOSE 8080

# Create startup script
COPY <<EOF /start.sh
#!/bin/sh
/cloud_sql_proxy -instances=${INSTANCE_CONNECTION_NAME}=tcp:5432 -dir=/cloudsql &
sleep 2
./main
EOF

RUN chmod +x /start.sh

# Run the start script
CMD ["/start.sh"]