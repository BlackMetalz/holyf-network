# Server
```bash
go run main.go -listen :18080
```

# Client
```bash
go run main.go -target 127.0.0.1:18080 -total 200000 -concurrency 1000 -timeout 1s
```