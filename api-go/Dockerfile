FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/collector ./cmd/collector

FROM alpine:3.20
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/collector /usr/local/bin/collector

CMD ["api"]
