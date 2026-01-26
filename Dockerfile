FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/lamigrate ./cmd/lamigrate

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /out/lamigrate /usr/local/bin/lamigrate
WORKDIR /work
ENTRYPOINT ["lamigrate"]
