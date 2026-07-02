FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /paperless-mcp .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /paperless-mcp /usr/local/bin/paperless-mcp
EXPOSE 8035
ENTRYPOINT ["paperless-mcp"]
