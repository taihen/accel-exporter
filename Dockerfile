# Stage 1: Build the application
FROM golang:1.21-alpine AS builder

# Set necessary environment variables
ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /app

# Create a non-root user and group in the builder stage
# Although not strictly necessary for the build itself,
# it helps ensure consistent file ownership if needed later.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Download Go modules first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application
# Ensure the output binary is owned by the non-root user
RUN go build -o accel-exporter ./cmd/accel-exporter && \
    chown appuser:appgroup accel-exporter

# Stage 2: Create the final lightweight image
FROM alpine:latest

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Install necessary certificates
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /home/appuser

# Copy the binary from the builder stage
# Ensure the binary is owned by the non-root user
COPY --from=builder --chown=appuser:appgroup /app/accel-exporter .

# Switch to the non-root user
USER appuser

# Expose the application port
EXPOSE 9101

# Set the entrypoint
ENTRYPOINT ["./accel-exporter"]
