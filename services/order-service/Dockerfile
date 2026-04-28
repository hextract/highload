FROM golang:1.22-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /order ./cmd/order

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /order /order
EXPOSE 8082
USER nobody
ENTRYPOINT ["/order"]
