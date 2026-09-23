FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /akim .
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D akim
WORKDIR /app
RUN mkdir .local && chown -R akim:akim /app
COPY --from=build /akim /app/akim
USER akim
EXPOSE 3000
CMD ["/app/akim"]
