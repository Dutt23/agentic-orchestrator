# Build stage - Use standard Debian-based image (more reliable than Alpine)
FROM python:3.11-slim AS builder

WORKDIR /build

# Install build dependencies (for psutil and other packages with C extensions)
# With retry logic for network reliability
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    python3-dev \
    && rm -rf /var/lib/apt/lists/* || \
    (echo "Retrying apt install..." && sleep 3 && apt-get update && apt-get install -y --no-install-recommends gcc python3-dev && rm -rf /var/lib/apt/lists/*)

# Copy requirements first (better caching)
COPY cmd/agent-runner-py/requirements.txt .

# Install dependencies and upgrade certifi for SSL
RUN pip install --no-cache-dir --upgrade pip certifi && \
    pip install --no-cache-dir -r requirements.txt

# Runtime stage
FROM python:3.11-slim

WORKDIR /app

# Install runtime dependencies only
# With retry logic for network reliability
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    openssl \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy installed packages from builder
COPY --from=builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin

# Copy application code
COPY cmd/agent-runner-py/ .

# Copy common schema files (needed for workflow validation)
COPY common/schema/ /common/schema/

# Create non-root user
RUN groupadd -g 1000 app && \
    useradd -u 1000 -g app -m -s /bin/bash app && \
    chown -R app:app /app && \
    chown -R app:app /common

USER app

# Set environment variables for SSL/TLS
# Point Python libraries to system CA certificates and certifi bundle
ENV SSL_CERT_FILE=/usr/local/lib/python3.11/site-packages/certifi/cacert.pem \
    REQUESTS_CA_BUNDLE=/usr/local/lib/python3.11/site-packages/certifi/cacert.pem \
    CURL_CA_BUNDLE=/usr/local/lib/python3.11/site-packages/certifi/cacert.pem

EXPOSE 8086

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD python -c "import sys; sys.exit(0)" || exit 1

CMD ["python", "-u", "main.py"]
