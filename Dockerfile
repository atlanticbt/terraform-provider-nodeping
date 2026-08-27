FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o terraform-provider-nodeping .

FROM alpine:latest AS runtime
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/terraform-provider-nodeping .
ENTRYPOINT ["./terraform-provider-nodeping"]

FROM golang:1.23-alpine AS test
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go mod tidy
CMD ["go", "test", "-v", "./..."]
