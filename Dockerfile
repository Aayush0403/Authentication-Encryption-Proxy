FROM golang:1.19-buster as builder

WORKDIR /app

COPY go.* ./

RUN go mod download

COPY . ./

RUN go build -mod=readonly -v -o server

FROM golang:1.18-buster AS build
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/server /app/server

CMD ["/app/server"]
