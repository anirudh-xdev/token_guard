# Build
FROM golang:1.21-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tokenguard ./cmd/tokenguard

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/tokenguard /app/tokenguard
COPY pricing.json /app/pricing.json
ENV TOKENGUARD_PRICING_FILE=/app/pricing.json
ENV TIKTOKEN_CACHE_DIR=/tmp/tiktoken-cache
EXPOSE 8080
# Render sets PORT; TokenGuard binds 0.0.0.0:$PORT when TOKENGUARD_LISTEN_ADDR is unset.
CMD ["/app/tokenguard"]
