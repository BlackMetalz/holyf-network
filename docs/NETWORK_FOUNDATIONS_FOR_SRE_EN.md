# Network Foundations For SRE (EN)

This document is not trying to teach full TCP/networking like a textbook.

Its goal is to:

- help you read `holyf-network` without getting lost in states like `TIME_WAIT`, `CLOSE_WAIT`, and `SYN_RECV`
- give you a practical mental model for narrowing incidents quickly
- stay operator/SRE-focused instead of diving into theory you do not need during triage

If you are new to TCP states, read this first and then come back to:

- `docs/USER_METRICS_GUIDE_EN.md`

Once the basics are clear and you want a 1-page incident reference:

- `docs/INCIDENT_MENTAL_CHECKLIST_EN.md`

## 1) 5 mental models that matter most

### 1. A TCP state is the local host's point of view

The TCP state you see on host A is how **host A** sees that connection.

This matters a lot:

- `CLOSE_WAIT` on the server usually means:
  - the client closed first
  - the server kernel already received the `FIN`
  - but the server application still has not called `close()` on its socket
- `TIME_WAIT` on the server usually means:
  - the server already moved through the close path and is waiting for cleanup
  - it does not automatically mean “the app is leaking sockets”

Short version:

- the same connection can show **different states on the client and the server**

### 2. TCP close is a two-sided process, not a single instant

A lot of people initially think `close` is one instant action. It is not.

When one side closes first, the other side still has to:

- receive the `FIN`
- process any remaining data
- call `close()`
- ACK / FIN in the right phase

That is why states like these exist:

- `CLOSE_WAIT`
- `FIN_WAIT1`
- `TIME_WAIT`

### 3. Queue values are local backlog on this host

`Send-Q` and `Recv-Q` in `holyf-network` are backlog snapshots on the **local host**, not queue values from the peer.

- High `Send-Q`:
  - this host still has data that has not fully left / has not been ACKed
  - that can mean downstream slowness, loss, congestion, or a slow receiver
- High `Recv-Q`:
  - this host already received data, but the application has not read it fast enough
  - that often points to slow local consumption

### 4. Retrans is a path-quality signal, not an app verdict

High `Retrans` usually points to:

- packet loss
- congestion
- poor path quality
- NIC or network-path issues

It does **not automatically prove** an app bug.

To make the right call, correlate it with:

- NIC `Errors/Drops`
- `Send-Q`
- `Recv-Q`
- `TIME_WAIT` / `CLOSE_WAIT`
- `Conntrack%`

### 5. Conntrack is not “how many app connections are open”

`Conntrack` is the kernel state-tracking table.

It exists for things like:

- NAT
- stateful firewalling
- kernel flow tracking

It is not the same thing as:

- the app's socket count
- the app's business-logic connection count
- outbound ephemeral port usage

So:

- high `Conntrack%` = kernel state-table pressure
- it does not automatically mean the app is leaking sockets

## 2) TCP states you should learn first

You do not need every TCP state. For day-to-day SRE work, these 5 are enough.

### `ESTABLISHED`

Meaning:

- the connection is active and working normally

When to worry:

- the count is abnormally high
- or it comes with persistently high `Send-Q` / `Recv-Q`

### `TIME_WAIT`

Rough meaning:

- the connection is logically closed
- this host is still holding the state for cleanup / late-packet safety

Common when:

- there are many short-lived connections
- an app or client opens and closes very quickly

When to worry:

- it rises sharply
- it dominates the state mix
- it comes with obvious churn

Think first about:

- short-lived connection storms
- clients not reusing connections
- a request/close pattern that is too aggressive

Do not jump straight to:

- “the app is not closing sockets”

### `CLOSE_WAIT`

Rough meaning:

- the peer already said “I am done”
- the local kernel knows that
- but the local app still has not closed its side

When to worry:

- it keeps rising
- it sticks around
- it owns a meaningful number of sockets

Think first about:

- a bug in the local app close path
- missing `close()` / `defer close()`
- blocked goroutines / threads

This state very often points to:

- **app-side socket leak / cleanup bug**

### `SYN_RECV`

Rough meaning:

- this host received a `SYN`
- replied to it
- but the handshake did not complete yet

When to worry:

- it spikes suddenly
- backlog pressure is visible
- it looks like incomplete handshakes are piling up

Think first about:

- SYN flood
- clients connecting but not completing the handshake
- backlog pressure

### `FIN_WAIT1`

Rough meaning:

- the local host already started closing
- but the peer has not ACKed / completed the close path yet

When to worry:

- it grows meaningfully
- cleanup is not progressing

Think first about:

- a slow close handshake
- slow peer ACK behavior
- cleanup lag

## 3) The shortest possible close-path mental model

### Peer closes first

```text
peer sends FIN
-> local enters CLOSE_WAIT
-> local app calls close()
-> local sends FIN
-> local waits for the final ACK
```

If you see a lot of `CLOSE_WAIT`:

- the peer already did its part
- the local app still has not closed the socket

### Local closes first

```text
local starts close
-> FIN_WAIT1
-> ...
-> TIME_WAIT
-> cleanup
```

If you see a lot of `TIME_WAIT`:

- first suspect short-lived connections / close churn
- not an app leak by default

## 4) How to read queue values correctly

### `Send-Q`

Practical meaning:

- local data wants to go out but has not fully left / been ACKed yet

Fast correlation:

- high `Send-Q` + high `Retrans`:
  - suspect path problems / loss / congestion
- high `Send-Q` + low `Retrans`:
  - suspect a slow peer or slow downstream more than loss

### `Recv-Q`

Practical meaning:

- the kernel already received data, but the local app has not drained it yet

Fast correlation:

- high `Recv-Q` that persists:
  - suspect slow local consumption
  - check CPU, goroutines/threads, read loops, and backpressure inside the app

## 5) Retrans: what it should mean to you

`Retrans` means the sender had to send a segment again.

It usually points to:

- packet loss
- congestion
- queueing / high path latency
- NIC / driver / path issues

Inside `holyf-network`:

- if you see `LOW SAMPLE`, **do not overreact**
- it means the current sample is too small to trust the retrans verdict yet

Treat retrans as a strong signal only when:

- it is no longer `LOW SAMPLE`
- it stays elevated
- and it correlates with queues, NIC errors, or app symptoms

## 6) Conntrack: how to read it correctly

In this app, the Conntrack section (inside `System Health` panel) should be treated as a **pressure indicator**.

Look at:

- `Used / Max`
- `Conntrack%`
- `Drops` when it is non-zero

Fast interpretation:

- high `Conntrack%`:
  - the kernel state table is under pressure
- `Drops > 0`:
  - the kernel is failing to insert new flows
  - this is a hard-failure signal

Do not confuse it with:

- app socket leaks
- service-level connection ownership

## 7) How to read holyf-network in 60 seconds

### Step 1: look at `Diagnosis`

`Diagnosis` answers quickly:

- what is most prominent right now
- whether this looks more like path trouble, app close-path trouble, or conntrack pressure

But remember:

- v1 is host-global
- it does not scope itself to your current filter/search

### Step 2: look at `Connection States`

Ask:

- which state dominates?
- is `Retrans` trustworthy yet, or still `LOW SAMPLE`?
- is `Conntrack%` high?

### Step 3: look at `Top Connections`

If you need a concrete flow:

- use `View=CONN`

If you need a pattern:

- use `View=GROUP`
- use `Selected Detail` to see the full grouped state breakdown (`States: ...`)

### Step 4: look at `Selected Detail`

The footer preview helps you understand:

- what the selected row actually represents
- what `Enter` / `k` would target

In `GROUP`, this matters a lot because the action still resolves to:

- `peer + local port`

## 8) 5 practical patterns

### Pattern 1: `TIME_WAIT` storm

Symptoms:

- `TIME_WAIT` is very high
- `Diagnosis` says `TIME_WAIT churn`
- `GROUP` shows concentration on one service/port

Interpretation:

- short-lived connections are opening and closing aggressively

Think next about:

- are clients reusing connections?
- is the service closing too quickly after every request?
- is a load balancer / health check / burst creating churn?

Check next:

```bash
ss -tan | awk '{print $1}' | sort | uniq -c | sort -nr
ss -tanp | grep ':18080'
```

### Pattern 2: `CLOSE_WAIT` leak

Symptoms:

- `CLOSE_WAIT` keeps growing
- it sticks around
- `Diagnosis` says `CLOSE_WAIT pressure`

Interpretation:

- the peer already closed
- the local app still did not close its socket

Think next about:

- a bug in the close path
- blocked goroutines/threads
- missing cleanup

Check next:

```bash
ss -tanp | grep CLOSE-WAIT
lsof -p <pid> | head
```

### Pattern 3: high retrans

Symptoms:

- `Diagnosis` says `TCP retrans is high`
- `Retrans` is no longer `LOW SAMPLE`

Interpretation:

- this is now a strong path-quality signal

Think next about:

- loss
- congestion
- NIC / driver
- unstable route/path

Check next:

```bash
ip -s link show dev eth0
ss -tin
awk '/^Tcp:/{if(!h){h=$0;next} v=$0; print h; print v; exit}' /proc/net/snmp
```

### Pattern 4: conntrack pressure

Symptoms:

- `Conntrack%` is high
- or `Drops > 0`
- `Diagnosis` says `Conntrack pressure high` or `Conntrack drops active`

Interpretation:

- the kernel state table is being pushed toward its limit

Think next about:

- heavy flow churn
- lots of NAT/firewall state
- `nf_conntrack_max` being too low

Check next:

```bash
cat /proc/sys/net/netfilter/nf_conntrack_count
cat /proc/sys/net/netfilter/nf_conntrack_max
conntrack -S
```

### Pattern 5: interface traffic is high but Top is unclear

Symptoms:

- `Interface Stats` is high
- but `Top Connections` does not show an obvious heavy row

Interpretation:

- traffic may be too short-lived between samples
- or flow visibility may still be incomplete

Think next about:

- reducing the interval
- refreshing for more samples
- filtering by port

Check next:

```bash
ss -ntp
conntrack -L -p tcp | head -n 30
```

## 9) 6 minimum commands worth remembering

### 1. View the overall state mix

```bash
ss -tan
```

### 2. View process + state

```bash
ss -tanp
```

### 3. View queue / tcp_info detail

```bash
ss -tin
```

### 4. View conntrack counters

```bash
conntrack -S
```

### 5. View NIC stats

```bash
ip -s link show dev eth0
```

### 6. View TCP SNMP counters

```bash
awk '/^Tcp:/{if(!h){h=$0;next} v=$0; print h; print v; exit}' /proc/net/snmp
```

## 10) Mental checklist when an incident is active

If you are overloaded, just ask yourself these 5 questions:

1. Is the main problem a TCP state issue, a path-quality issue, or state-table pressure?
2. Is this state the local host's point of view, or am I accidentally thinking from the peer's side?
3. Are queues pointing to a slow local app or a slow network path?
4. Is retrans trustworthy yet, or still under `LOW SAMPLE`?
5. Is one peer/service clearly dominating?

If you can answer those 5 questions, you already have enough foundation to use `holyf-network` effectively.
