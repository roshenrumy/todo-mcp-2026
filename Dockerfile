FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /todo-mcp ./cmd/todo-mcp
FROM gcr.io/distroless/base-debian12
COPY --from=build /todo-mcp /todo-mcp
ENTRYPOINT ["/todo-mcp"]
