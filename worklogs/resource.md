# TCP

### TCP full-duplex

TCP is full-duplex, so either the client or the server can call `close()`.

Basic flow, for example Server will call `close()` first.

1. Server call `close()`
- Send `FIN`, tcp state will be `FIN-WAIT-1`

2. Client received `FIN` 
- send `ACK`
- Client state will be `CLOSE-WAIT`

Server received `ACK`, tcp state will be `FIN-WAIT-2`

3. Client call `close()` also
- Send `FIN`, TCP state is `LAST-ACK`

Server take `FIN`
- Send `ACK`
- TCP state into `TIME-WAIT`

Remember:
- The side that calls `close()` first goes through FIN-WAIT states.
- The side that receives FIN first goes through CLOSE-WAIT.
- The side that sends the final ACK (state: `LAST-ACK`) enters TIME-WAIT.

### Flow to remember
- Who call `close()` first
```
ESTABLISHED
→ FIN-WAIT-1
→ FIN-WAIT-2
→ TIME-WAIT
→ CLOSED
```

- Who received `FIN` first
```
ESTABLISHED
→ CLOSE-WAIT
→ LAST-ACK
→ CLOSED
```

### TCP States
Resource: https://datatracker.ietf.org/doc/html/rfc9293.html (published 2022) (search "Briefly the meanings of the states are")

- `LISTEN`: opening, waiting for connection
- `ESTAB`: active connection
- `TIME-WAIT`: done close, waiting for timeout
- `SYN-SENT`: on going connect, but not finish handshake
- `CLOSE-WAIT`: The peer (the app that connects to your app) has already closed the connection, but your app has not closed it yet, so the socket remains in `CLOSE-WAIT` until the application closes it.
- `LAST-ACK`: App local/your app already closed, waiting for final ACK from peer.
- `FIN-WAIT-1/2, LAST-ACK, CLOSING`: on going close

