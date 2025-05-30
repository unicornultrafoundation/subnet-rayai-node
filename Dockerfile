ARG BASE_IMAGE="rayproject/ray:latest"

# Stage 1: Build Golang application
FROM golang:1.24 AS builder

WORKDIR /app


# Copy go.mod and go.sum first for better caching
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the rest of the code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rayai-node ./cmd/rayai-node

# Stage 2: Create final image with Ray
FROM ${BASE_IMAGE}

# Install required system dependencies
USER root
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Define working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/rayai-node .

# Create a non-root user for running the app
RUN groupadd -r rayai && useradd -r -g rayai rayai
RUN chown -R rayai:rayai /app
USER rayai

# Expose necessary ports
EXPOSE 3333 6379 8265

# Set environment variables
ENV API_PORT=3333
ENV RAY_BIN_PATH=ray
ENV LOG_LEVEL=info
ENV ALLOWED_IPS=*

# Command to run when container starts
CMD ["./rayai-node"]
