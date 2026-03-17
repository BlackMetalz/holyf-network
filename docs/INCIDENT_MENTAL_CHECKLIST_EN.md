# Incident Mental Checklist (EN)

This document is for the moment when an alert is firing or you are SSHed into a host and need a fast frame.

Its goal is to:

- stop you from getting overwhelmed by too many panels/signals
- force the investigation back to the 5-6 questions that matter most
- help you separate app issues, network-path issues, and conntrack/state-table pressure

If parts of this checklist still feel unclear, read:

- `docs/NETWORK_FOUNDATIONS_FOR_SRE_EN.md`

## 30-second checklist

### 1. What is the most prominent issue right now?

Look at `Diagnosis` first.

Ask yourself:

- is it talking about `TIME_WAIT`?
- `CLOSE_WAIT`?
- `Retrans`?
- `Conntrack`?

If `Diagnosis` is unclear or `LOW SAMPLE` is still active, do not stop there. Continue through the other panels.

### 2. Is this mainly an app issue, a network-path issue, or kernel state-table pressure?

Quick mapping:

- high `CLOSE_WAIT`:
  - usually points to the local app not closing sockets
- high `Retrans`:
  - usually points to path quality / packet loss / congestion / NIC issues
- high `Conntrack%` or `Drops > 0`:
  - usually points to kernel state-table pressure
- high `TIME_WAIT`:
  - usually points to short-lived connection churn

### 3. Is this state the local host's point of view, or am I accidentally thinking from the peer's side?

Remind yourself:

- `CLOSE_WAIT` = the peer closed first, local app has not closed yet
- `TIME_WAIT` = the local host is holding cleanup state after close
- do not look at a state and unconsciously reason from the peer's intent

### 4. Are queues pointing to a slow app or a slow network path?

Look at `Top Connections`:

- high `Send-Q`:
  - data is not getting out cleanly yet
  - if retrans is also high, suspect path trouble
  - if retrans is low, suspect a slow downstream/peer
- high `Recv-Q`:
  - suspect the local app is reading too slowly

### 5. Is retrans trustworthy yet?

If `Retrans` still shows `LOW SAMPLE`:

- do not conclude “network issue” yet
- get more sample or more traffic first

Treat retrans as a strong signal only when:

- it is no longer `LOW SAMPLE`
- and it stays elevated

### 6. Is conntrack a side symptom or the actual bottleneck?

Look at the `Conntrack` panel:

- `Used / Max`
- `Conntrack%`
- `Drops` when non-zero

Fast conclusion:

- high `%` but no `Drops` yet:
  - pressure is building
- `Drops` present:
  - that is a hard-failure signal, prioritize it

### 7. Is one peer/service clearly dominating?

Switch to `View=GROUP` and look at:

- `CONNS`
- `PORTS`
- `Selected Detail -> States`

Use `Selected Detail` to understand:

- what the row actually represents
- which `peer + local port` `Enter` / `k` would target

## If you see this pattern, think this first

### High `TIME_WAIT`

Think first:

- short-lived connections are churning hard

Check next:

- are clients reusing connections?
- are health checks / bursts causing churn?
- which port is this concentrated on?

### High `CLOSE_WAIT`

Think first:

- the local app may not be closing sockets

Check next:

- which process owns most rows?
- is there a cleanup-path bug?

### High `SYN_RECV`

Think first:

- handshakes are not completing

Check next:

- backlog pressure?
- SYN flood?
- clients that connect but never finish?

### High `Retrans`

Think first:

- network path / loss / congestion / NIC issue

Check next:

- `ip -s link show dev <iface>`
- `ss -tin`
- is `Send-Q` also high?

### High `Conntrack%` or `Drops > 0`

Think first:

- the kernel state table is under pressure

Check next:

- flow churn
- NAT/firewall state
- `nf_conntrack_max`

## 5 quick cross-check commands

```bash
ss -tan
ss -tanp
ss -tin
conntrack -S
ip -s link show dev eth0
```

## 3 common thinking mistakes

### 1. Seeing high `TIME_WAIT` and concluding the app is leaking sockets

That is not the default interpretation.

`TIME_WAIT` is more often about:

- churn
- short-lived connections

### 2. Seeing high retrans and concluding “app bug”

That is not enough.

Retrans is usually a path-quality signal first.

### 3. Seeing high Conntrack and assuming the app owns too many sockets

Not necessarily.

Conntrack is kernel state tracking, not app socket ownership.

## When overloaded, just ask these 5 questions

1. Is the main problem a state issue, a path issue, or state-table pressure?
2. Is this state the local host's point of view or the peer's?
3. Are queues pointing to a slow local app or a slow network path?
4. Is retrans trustworthy yet?
5. Is one peer/service dominating?

If you can answer those 5 questions, you usually have enough to decide the next investigation step.
