# Chef stage - pre-compute dependencies
FROM rust:1.75-alpine AS chef

RUN apk update && apk add --no-cache musl-dev openssl-dev

# Install cargo-chef
RUN cargo install cargo-chef --locked

WORKDIR /build

# Planner stage - analyze dependencies
FROM chef AS planner

COPY cmd/aob-cli/Cargo.toml cmd/aob-cli/Cargo.lock ./
COPY cmd/aob-cli/src ./src

RUN cargo chef prepare --recipe-path recipe.json

# Builder stage - build dependencies and app
FROM chef AS builder

COPY --from=planner /build/recipe.json recipe.json

# Build dependencies (cached layer!)
RUN cargo chef cook --release --recipe-path recipe.json

# Copy source and build app
COPY cmd/aob-cli/Cargo.toml cmd/aob-cli/Cargo.lock ./
COPY cmd/aob-cli/src ./src

# Build with aggressive optimizations
ENV RUSTFLAGS="-C target-feature=+crt-static -C link-arg=-s"

RUN cargo build --release --locked && \
    strip /build/target/release/aob

# Runtime stage
FROM alpine:3.19

WORKDIR /app

RUN apk update && apk add --no-cache ca-certificates libgcc

COPY --from=builder /build/target/release/aob .

RUN addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app app && \
    chown app:app /app/aob && \
    chmod +x /app/aob

USER app

ENTRYPOINT ["./aob"]
CMD ["--help"]
