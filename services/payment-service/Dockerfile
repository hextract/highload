FROM golang:1.22-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /payment ./cmd/payment

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /payment /payment
EXPOSE 8083
USER nobody
ENTRYPOINT ["/payment"]
