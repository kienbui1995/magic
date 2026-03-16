FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY core/ ./core/
RUN cd core && go build -o /magic ./cmd/magic

FROM alpine:3.20
COPY --from=builder /magic /usr/local/bin/magic
EXPOSE 8080
ENV MAGIC_PORT=8080
ENTRYPOINT ["magic", "serve"]
