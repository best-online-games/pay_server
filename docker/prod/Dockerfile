# Step 1: Modules caching
FROM golang:1.23.0-alpine as modules
COPY go.mod go.sum /modules/
WORKDIR /modules
RUN go mod download

# Step 2: Builder
FROM golang:1.23.0-alpine as builder
COPY --from=modules /go/pkg /go/pkg
COPY . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/app ./cmd/service

# Step 3: Final
FROM alpine:3.20
RUN apk add --no-cache bash openssl
COPY --from=builder /bin/app /app
ENTRYPOINT ["/app"]
