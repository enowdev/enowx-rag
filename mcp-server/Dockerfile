# Stage 1: Build the React SPA
FROM node:24-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build the Go binary (with embedded SPA)
FROM golang:1.26-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built SPA into web/dist so embed.FS picks it up
COPY --from=web-builder /web/dist ./web/dist
RUN go build -trimpath -o enowx-rag ./cmd/mcp-server

# Stage 3: Minimal runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /build/enowx-rag .
EXPOSE 7777
ENTRYPOINT ["./enowx-rag"]
