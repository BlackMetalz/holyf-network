# AGENT_SYNC — Multi-agent coordination board

> Source of truth for: plan, task split, questions/answers, progress, and reviewer punch-list.
> Rule: Every agent MUST read this file first, then write back updates before finishing a turn.
> Section ownership: Planner writes Sections 0-1, DEV_A writes Section 2, DEV_B writes Section 3, Coordinator writes Sections 4 and 6, Reviewer writes Section 5.

---

## 0) Current Assignment (from User)
- **User request:** TODO (planner fills)
- **Hard constraints:** TODO (planner fills)
- **Out of scope:** TODO (planner fills)

### 0.1 Run Control (planner fills, coordinator enforces)
| Role | Heartbeat interval | Stale timeout | Hard timeout |
| --- | --- | --- | --- |
| Planner | 60s | 3m | 5m |
| DEV_A | 2m | 6m | 20m |
| DEV_B | 2m | 6m | 20m |
| Reviewer | 90s | 4m | 8m |

- **Coordinator:** @coordinator (main agent)
- **Last coordinator check:** YYYY-MM-DD HH:MM
- **Recovery policy:** stale ping -> mark `STALE` -> interrupt at hard-timeout -> spawn backfill logger to update missing section -> continue pipeline.
- **Stale reason codes:** `INTERRUPTED` / `WAIT_TIMEOUT` / `TOOL_TIMEOUT` / `BLOCKED_EXTERNAL` / `UNKNOWN`
- **Checkpoint policy:** every role heartbeat must add one `[HB][YYYY-MM-DD HH:MM]` note in its section with current command/result/next step.
- **Long command policy:** before command expected >60s, log `[RUNNING][timestamp] <cmd>`; after it exits, log `[DONE][timestamp] exit=<code> summary`.

---

## 1) Plan (Planner output)
**Planner:** @planner  
**Status:** DRAFT / IN_PROGRESS / LOCKED  
**Last updated:** YYYY-MM-DD HH:MM

### 1.1 Assumptions
- [ ] A1:
- [ ] A2:

### 1.2 Step-by-step Plan
1) ...
2) ...
3) ...

### 1.3 Task Breakdown
**DEV_A**
- [ ] Task A1: (files/area)
- [ ] Task A2:

**DEV_B**
- [ ] Task B1: (tests/integration/edge cases)
- [ ] Task B2:

### 1.4 Acceptance Criteria (Quality Gate)
- [ ] AC1:
- [ ] AC2:
- [ ] AC3:

### 1.5 Risks / Gotchas
- R1:
- R2:

### 1.6 Coordination Notes
- Section ownership confirmed: YES / NO
- Timeouts/heartbeat reviewed against Section 0.1: YES / NO
- Reviewer preconditions (Sections 2 and 3 set to `DONE`): YES / NO

---

## 2) DEV_A Work Log
**Owner:** @dev_a  
**Status:** NOT_STARTED / IN_PROGRESS / BLOCKED / STALE / DONE  
**Started at:** YYYY-MM-DD HH:MM  
**Last heartbeat:** YYYY-MM-DD HH:MM  
**Current step:** ...
**Branch/worktree:** (optional)

### 2.1 Progress
- [ ] A1:
- [ ] A2:

### 2.2 Notes (implementation decisions)
- Decision 1:
- Decision 2:
- Files/functions touched:
- Commands run + results:
- Next steps:

### 2.3 Questions for DEV_B (blocking/clarification protocol)
- Format for open question: `[OPEN][Q-001][YYYY-MM-DD HH:MM] question text`
- Format for resolved confirmation: `[RESOLVED][Q-001][YYYY-MM-DD HH:MM] resolution note`
- Questions:
  - None

### 2.4 Heartbeat Journal (liveness evidence)
- Format:
  - `[HB][YYYY-MM-DD HH:MM] step=<short step> next=<next action>`
  - `[RUNNING][YYYY-MM-DD HH:MM] <command>`
  - `[DONE][YYYY-MM-DD HH:MM] exit=<code> <short result>`
- Journal:
  - None

---

## 3) DEV_B Work Log
**Owner:** @dev_b  
**Status:** NOT_STARTED / IN_PROGRESS / BLOCKED / STALE / DONE  
**Started at:** YYYY-MM-DD HH:MM  
**Last heartbeat:** YYYY-MM-DD HH:MM  
**Current step:** ...
**Branch/worktree:** (optional)

### 3.1 Progress
- [ ] B1:
- [ ] B2:

### 3.2 Answers to DEV_A questions (reply protocol)
- Format for answer: `[ANSWERED][Q-001][YYYY-MM-DD HH:MM] answer text`
- Answers:
  - None

### 3.3 Test Plan
- Unit tests:
- Integration tests:
- Manual checks:
- Commands run + results:
- Edge-case notes:

### 3.4 Heartbeat Journal (liveness evidence)
- Format:
  - `[HB][YYYY-MM-DD HH:MM] step=<short step> next=<next action>`
  - `[RUNNING][YYYY-MM-DD HH:MM] <command>`
  - `[DONE][YYYY-MM-DD HH:MM] exit=<code> <short result>`
- Journal:
  - None

---

## 4) Integration / Merge Notes
- **Owner:** @coordinator
- **Last updated:** YYYY-MM-DD HH:MM
- Conflicts / dependencies:
- Stale triage:
  - Role:
  - Reason code:
  - Last heartbeat:
  - Last successful command/result:
  - Recovery owner + next action:
- Commands to run:
  - `...`
  - `...`
- Recovery actions taken (if any):
  - `...`

---

## 5) Reviewer Gate (Must reference Plan + Acceptance Criteria)
**Reviewer:** @reviewer  
**Status:** NOT_REVIEWED / CHANGES_REQUESTED / APPROVED  
**Reviewed against Plan version:** (timestamp from section 1)

### 5.1 Acceptance Criteria Results
- AC1: PASS / FAIL — note
- AC2: PASS / FAIL — note
- AC3: PASS / FAIL — note

### 5.2 Issues
**Blockers**
- [ ] B1:
- [ ] B2:

**Non-blockers**
- [ ] N1:
- [ ] N2:

### 5.3 Suggested Fixes (actionable)
- Patch idea 1:
- Patch idea 2:

### 5.4 Evidence Checklist
- [ ] Section 2 has command/test evidence
- [ ] Section 3 has command/test evidence
- [ ] Q/A protocol entries are resolved or explicitly none
- [ ] Review executed within timeout policy

---

## 6) Handoff Checklist (end of each agent turn)
- [ ] Updated this file with what I changed + next steps
- [ ] Linked files/paths/functions I touched
- [ ] Listed commands run + results (tests, build)
- [ ] Updated `Last heartbeat` during execution
- [ ] Marked final status (`DONE` / `BLOCKED` / `STALE`) before handoff
- [ ] Added stale reason code if status is `BLOCKED` or `STALE`
- [ ] Added heartbeat journal entries (`HB` + `RUNNING/DONE`) for long commands
