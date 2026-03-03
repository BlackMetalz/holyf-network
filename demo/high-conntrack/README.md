# Server
```bash
go run main.go -listen :18080
```

# Client
```bash
go run main.go -target 127.0.0.1:18080 -total 200000 -concurrency 1000 -timeout 1s
```

With little delay, with this we can see clearly in conntrack and CLOSE-WAIT state
```bash
go run main.go -total 1000 -concurrency 200 -hold 60s -delay 10ms
```