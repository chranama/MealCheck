FROM golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/mealcheck-server ./cmd/mealcheck-server

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
RUN groupadd -g 10001 mealcheck && useradd -u 10001 -g 10001 -M mealcheck \
    && mkdir -p /var/lib/mealcheck/artifacts \
    && chown -R 10001:10001 /var/lib/mealcheck
COPY --from=build /out/mealcheck-server /usr/local/bin/mealcheck-server
COPY data/ /opt/mealcheck/data/
COPY examples/ /opt/mealcheck/examples/
COPY schemas/ /opt/mealcheck/schemas/
COPY deploy/controller/api-entrypoint.sh /usr/local/bin/api-entrypoint
USER 10001:10001
WORKDIR /opt/mealcheck
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api-entrypoint"]
