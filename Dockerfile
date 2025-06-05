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

USER root
RUN apt-get update && apt-get install -y \
    ca-certificates \
    haproxy \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/rayai-node .


RUN groupadd -r rayai && useradd -r -g rayai rayai
RUN chown -R rayai:rayai /app
USER rayai

EXPOSE 3333 6379 10001 8265

ENV API_PORT=3333
ENV RAY_BIN_PATH=ray
ENV LOG_LEVEL=info

CMD ["/app/rayai-node"]