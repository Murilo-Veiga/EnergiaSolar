FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/collector ./cmd/collector
RUN CGO_ENABLED=0 go build -o /out/backfill-history ./cmd/backfill-history

FROM alpine:3.20
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/collector /usr/local/bin/collector
COPY --from=build /out/backfill-history /usr/local/bin/backfill-history

CMD ["api"]
