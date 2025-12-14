# ============================================
# UI Build stage
# ============================================
FROM node:20-slim AS ui-builder

WORKDIR /ui

# 1. Copy ALL source files first (including your local node_modules)
COPY ui/ ./

# 2. Force a clean install inside the container
# npm ci will automatically delete the local node_modules you just copied
# and reinstall the correct Linux binaries (fixing the Rollup error)
RUN npm ci

# 3. Now run the build
RUN npm run build

# ============================================
# Go Build stage
# ============================================
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled (required for onnxruntime_go)
RUN CGO_ENABLED=1 GOOS=linux go build -o skyclf ./cmd/server

# ============================================
# Runtime stage
# ============================================
FROM debian:bookworm-slim

# Install dependencies and ONNX Runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Download and install ONNX Runtime
ARG ORT_VERSION=1.23.2
RUN wget -q https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz \
    && tar xzf onnxruntime-linux-x64-${ORT_VERSION}.tgz \
    && cp onnxruntime-linux-x64-${ORT_VERSION}/lib/libonnxruntime.so* /usr/lib/ \
    && ldconfig \
    && rm -rf onnxruntime-linux-x64-${ORT_VERSION}*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/skyclf .

# Copy frontend dist from ui-builder
COPY --from=ui-builder /ui/dist ./ui/dist

# Create data directories
RUN mkdir -p /data/images /data/models /data/labels

# Environment
ENV SKYCLF_ADDR=:8080
ENV SKYCLF_DATA_DIR=/data
ENV SKYCLF_MODELS_DIR=/data/models
ENV SKYCLF_IMAGES_DIR=/data/images
ENV SKYCLF_LABELS_DB=/data/labels/labels.db
ENV SKYCLF_ORT_LIB=/usr/lib/libonnxruntime.so

EXPOSE 8080

CMD ["./skyclf"]