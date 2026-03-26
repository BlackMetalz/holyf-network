# Network Foundations For SRE (VI)

Doc này không cố dạy đầy đủ TCP/networking như textbook.

Mục tiêu của nó là:

- giúp bạn đọc `holyf-network` mà không bị mù trước các state như `TIME_WAIT`, `CLOSE_WAIT`, `SYN_RECV`
- cho bạn một mental model đủ tốt để khoanh vùng sự cố nhanh
- tập trung vào góc nhìn operator/SRE, không đi sâu vào theory không cần thiết

Nếu bạn mới với TCP state, hãy đọc doc này trước rồi quay lại:

- `docs/USER_METRICS_GUIDE_VI.md`

Khi đã hiểu nền tảng và cần một bản cực ngắn để dùng lúc đang incident:

- `docs/INCIDENT_MENTAL_CHECKLIST_VI.md`

## 1) 5 mental models quan trọng nhất

### 1. State là góc nhìn của local host

TCP state bạn thấy trên máy A là cách **máy A** nhìn connection đó.

Điều này rất quan trọng:

- `CLOSE_WAIT` trên server thường nghĩa là:
  - client đã đóng trước
  - kernel phía server đã nhận `FIN`
  - nhưng app phía server chưa `close()` socket
- `TIME_WAIT` trên server thường nghĩa là:
  - phía server đã đi qua phase close và đang chờ cleanup
  - không đồng nghĩa với “app leak socket”

Nói ngắn:

- cùng một connection, client và server có thể đang ở **state khác nhau**

### 2. TCP close là quá trình 2 chiều, không phải một cú “đóng” duy nhất

Nhiều người mới hay nghĩ `close` là xong ngay. Thực tế không phải vậy.

Một bên đóng trước thì bên kia còn phải:

- nhận `FIN`
- xử lý nốt data còn lại
- gọi `close()`
- ACK / FIN đúng phase

Vì vậy mới có các state như:

- `CLOSE_WAIT`
- `FIN_WAIT1`
- `TIME_WAIT`

### 3. Queue là backlog cục bộ trên host này

`Send-Q` và `Recv-Q` trong `holyf-network` là snapshot backlog ở **host local**, không phải queue của peer.

- `Send-Q` cao:
  - host này còn data chưa gửi xong / chưa được ACK
  - có thể do downstream chậm, loss, congestion, peer nhận chậm
- `Recv-Q` cao:
  - host này đã nhận data nhưng app chưa đọc kịp
  - thường nghi app consume chậm

### 4. Retrans là tín hiệu chất lượng đường truyền, không phải verdict về app

`Retrans` cao thường nói rằng:

- packet loss
- congestion
- path quality kém
- NIC / network path có vấn đề

Nó **không tự động kết luận** app bug.

Muốn kết luận đúng, phải nhìn thêm:

- `Errors/Drops` ở NIC
- `Send-Q`
- `Recv-Q`
- `TIME_WAIT` / `CLOSE_WAIT`
- `Conntrack%`

### 5. Conntrack không phải “số connection app đang mở”

`Conntrack` là bảng state tracking của kernel.

Nó tồn tại để phục vụ:

- NAT
- stateful firewall
- một số flow tracking logic của kernel

Nó không đồng nhất với:

- số socket của app
- số connection app giữ trong process
- số ephemeral port outbound

Vì vậy:

- `Conntrack%` cao = kernel state-table pressure
- không nhất thiết = app bị leak socket

## 2) TCP states bạn nên nhớ đầu tiên

Không cần nhớ hết mọi state. Với SRE, 5 state sau là đủ để làm việc hằng ngày.

### `ESTABLISHED`

Nghĩa là:

- kết nối đang hoạt động bình thường

Khi nào đáng lo:

- số lượng quá lớn bất thường
- hoặc đi kèm `Send-Q` / `Recv-Q` cao kéo dài

### `TIME_WAIT`

Nghĩa gần đúng:

- connection đã đóng logic xong
- host này đang giữ state thêm một lúc để tránh rác packet / nhầm sequence

Thường gặp khi:

- có nhiều kết nối ngắn hạn
- app/client mở rồi đóng rất nhanh

Khi nào đáng lo:

- tăng rất mạnh
- chiếm đa số state
- đi kèm churn cao

Thường nghĩ tới:

- short-lived connection storm
- client không reuse connection
- app pattern request/close quá dày

Không nên vội kết luận:

- “app không close socket”

### `CLOSE_WAIT`

Nghĩa gần đúng:

- peer đã nói “tôi đóng rồi”
- kernel local đã biết chuyện đó
- nhưng app local chưa đóng socket của mình

Khi nào đáng lo:

- tăng liên tục
- đứng lâu
- chiếm nhiều socket

Thường nghĩ tới:

- app local có bug ở close path
- code không `close()` / `defer close()` không chạy / goroutine kẹt

Đây là state rất hay gợi ý:

- **app-side socket leak / socket cleanup bug**

### `SYN_RECV`

Nghĩa gần đúng:

- host này đã nhận `SYN`
- đã trả lời
- nhưng handshake chưa complete

Khi nào đáng lo:

- tăng đột biến
- backlog pressure
- đi kèm dấu hiệu flood / incomplete handshake

Thường nghĩ tới:

- SYN flood
- client connect nhưng không hoàn tất handshake
- backlog queue căng

### `FIN_WAIT1`

Nghĩa gần đúng:

- host local đã bắt đầu close
- nhưng peer chưa ACK/finish đúng nhịp

Khi nào đáng lo:

- tăng nhiều
- cleanup không trôi

Thường nghĩ tới:

- close handshake bị chậm
- peer ACK chậm
- cleanup path lag

## 3) Close path cực ngắn để nhớ

### Peer đóng trước

```text
peer gửi FIN
-> local vào CLOSE_WAIT
-> app local close()
-> local gửi FIN
-> local chờ ACK cuối
```

Nếu bạn thấy nhiều `CLOSE_WAIT`:

- peer đã làm phần việc của họ
- local app chưa close socket

### Local đóng trước

```text
local bắt đầu close
-> FIN_WAIT1
-> ...
-> TIME_WAIT
-> cleanup
```

Nếu bạn thấy nhiều `TIME_WAIT`:

- thường nghi kết nối ngắn hạn / close churn
- không phải default symptom của app leak

## 4) Cách đọc queue cho đúng

### `Send-Q`

Nghĩa thực dụng:

- data local muốn đẩy đi nhưng chưa thoát hết / chưa được ACK

Kết hợp nhanh:

- `Send-Q` cao + `Retrans` cao:
  - nghi path xấu / loss / congestion
- `Send-Q` cao + `Retrans` thấp:
  - nghi peer đọc chậm hoặc downstream chậm hơn là loss

### `Recv-Q`

Nghĩa thực dụng:

- kernel đã nhận data nhưng app local chưa đọc hết

Kết hợp nhanh:

- `Recv-Q` cao kéo dài:
  - nghi app local xử lý đọc chậm
  - check CPU, goroutine/thread, read loop, backpressure trong app

## 5) Retrans: nên hiểu thế nào cho đúng

`Retrans` = sender phải gửi lại segment.

Nó thường gợi ý:

- packet loss
- congestion
- queueing / latency path cao
- vấn đề NIC / driver / path network

Trong `holyf-network`:

- nếu hiện `LOW SAMPLE` thì **đừng overreact**
- nghĩa là sample hiện tại chưa đủ lớn để verdict retrans tin cậy

Chỉ bắt đầu coi retrans là strong signal khi:

- sample đủ
- retrans tăng liên tục
- và tương quan với queue / NIC errors / symptom từ app

## 6) Conntrack: đọc thế nào cho đúng

Trong app này, phần Conntrack (trong panel `System Health`) nên được xem là **pressure indicator**.

Hãy nhìn:

- `Used / Max`
- `Conntrack%`
- `Drops` nếu khác `0`

Diễn giải nhanh:

- `Conntrack%` cao:
  - bảng state tracking đang căng
- `Drops > 0`:
  - kernel bắt đầu không insert được flow mới
  - đây là hard-failure signal

Đừng nhầm nó với:

- app socket leak
- số connection business logic của service

## 7) Cách đọc holyf-network trong 60 giây

### Bước 1: nhìn `Diagnosis`

`Diagnosis` trả lời nhanh:

- chuyện nổi bật nhất là gì
- nghi hướng network/path, app close path, hay conn churn

Nhưng nhớ:

- v1 là host-global
- không bị scope theo filter/search hiện tại

### Bước 2: nhìn `Connection States`

Hỏi:

- state nào đang dominate?
- `Retrans` có đáng tin chưa hay đang `LOW SAMPLE`?
- `Conntrack%` có cao không?

### Bước 3: nhìn `Top Connections`

Nếu cần xem flow cụ thể:

- ở `View=CONN`: xem từng connection

Nếu cần xem mẫu hình:

- ở `View=GROUP`: xem theo `(peer, process)`
- dùng `Selected Detail` để xem full state breakdown của group (`States: ...`)

### Bước 4: nhìn `Selected Detail`

Footer preview giúp bạn hiểu:

- row đang chọn thật ra là gì
- nếu bấm `Enter` / `k` thì app sẽ target gì

Ở `GROUP`, phần này rất quan trọng vì action cuối cùng vẫn resolve về:

- `peer + local port`

## 8) 5 pattern thực chiến

### Pattern 1: `TIME_WAIT` storm

Triệu chứng:

- `TIME_WAIT` rất cao
- `Diagnosis` nói `TIME_WAIT churn`
- `GROUP` cho thấy tập trung vào một port/service

Diễn giải:

- nhiều kết nối ngắn hạn đang mở/đóng liên tục

Nghĩ tiếp:

- client có đang reuse connection không?
- service có đang đóng quá nhanh sau mỗi request không?
- có load balancer / health check / burst traffic nào tạo churn không?

Check tiếp:

```bash
ss -tan | awk '{print $1}' | sort | uniq -c | sort -nr
ss -tanp | grep ':18080'
```

### Pattern 2: `CLOSE_WAIT` leak

Triệu chứng:

- `CLOSE_WAIT` tăng dần
- đứng lâu
- `Diagnosis` nói `CLOSE_WAIT pressure`

Diễn giải:

- peer đã close rồi
- app local chưa close socket

Nghĩ tiếp:

- bug ở close path
- goroutine/thread kẹt
- missing cleanup

Check tiếp:

```bash
ss -tanp | grep CLOSE-WAIT
lsof -p <pid> | head
```

### Pattern 3: retrans cao

Triệu chứng:

- `Diagnosis` nói `TCP retrans is high`
- `Retrans` không còn `LOW SAMPLE`

Diễn giải:

- đây là strong signal của path quality issue

Nghĩ tiếp:

- loss
- congestion
- NIC / driver
- route/path bất ổn

Check tiếp:

```bash
ip -s link show dev eth0
ss -tin
awk '/^Tcp:/{if(!h){h=$0;next} v=$0; print h; print v; exit}' /proc/net/snmp
```

### Pattern 4: conntrack pressure

Triệu chứng:

- `Conntrack%` cao
- hoặc `Drops > 0`
- `Diagnosis` nói `Conntrack pressure high` hoặc `Conntrack drops active`

Diễn giải:

- kernel state table đang bị đẩy tới ngưỡng

Nghĩ tiếp:

- flow churn quá lớn
- NAT/firewall state nhiều
- capacity `nf_conntrack_max` thấp

Check tiếp:

```bash
cat /proc/sys/net/netfilter/nf_conntrack_count
cat /proc/sys/net/netfilter/nf_conntrack_max
conntrack -S
```

### Pattern 5: interface cao nhưng Top không rõ

Triệu chứng:

- `Interface Stats` cao
- nhưng `Top Connections` không có row nào quá rõ

Diễn giải:

- traffic có thể quá ngắn hạn giữa các sample
- hoặc flow visibility chưa đủ tốt

Nghĩ tiếp:

- giảm interval
- refresh thêm sample
- filter theo port

Check tiếp:

```bash
ss -ntp
conntrack -L -p tcp | head -n 30
```

## 9) 6 lệnh tối thiểu nên nhớ

### 1. Xem state tổng quát

```bash
ss -tan
```

### 2. Xem process + state

```bash
ss -tanp
```

### 3. Xem queue / tcp_info chi tiết hơn

```bash
ss -tin
```

### 4. Xem conntrack counters

```bash
conntrack -S
```

### 5. Xem NIC stats

```bash
ip -s link show dev eth0
```

### 6. Xem TCP SNMP counters

```bash
awk '/^Tcp:/{if(!h){h=$0;next} v=$0; print h; print v; exit}' /proc/net/snmp
```

## 10) Checklist tư duy khi đang incident

Nếu bạn bị ngợp, chỉ cần tự hỏi 5 câu này:

1. Vấn đề nổi bật nhất là **state**, **path quality**, hay **state-table pressure**?
2. State đó là góc nhìn của **local host** hay mình đang vô thức nghĩ theo peer?
3. `Send-Q` / `Recv-Q` đang nói app local chậm hay network path chậm?
4. `Retrans` có đủ sample để tin chưa?
5. `Conntrack` đang là symptom phụ, hay là bottleneck thật?

Nếu trả lời được 5 câu này, bạn đã qua phần “foundation” đủ để dùng `holyf-network` rất hiệu quả rồi.
