# Incident Mental Checklist (VI)

Doc này dành cho lúc đang có alert hoặc đang SSH vào host để nhìn nhanh.

Mục tiêu:

- không bị ngợp bởi quá nhiều panel/signal
- ép mình quay về đúng 5-6 câu hỏi quan trọng nhất
- giúp phân biệt nhanh: app issue, network path issue, hay conntrack/state pressure

Nếu bạn còn thấy nhiều chỗ trong checklist này chưa rõ, đọc thêm:

- `docs/NETWORK_FOUNDATIONS_FOR_SRE_VI.md`

## Checklist 30 giây

### 1. Vấn đề nổi bật nhất đang là gì?

Nhìn `Diagnosis` trước.

Tự hỏi:

- nó đang nói về `TIME_WAIT`?
- `CLOSE_WAIT`?
- `Retrans`?
- `Conntrack`?

Nếu `Diagnosis` chưa rõ hoặc `LOW SAMPLE`, đừng dừng ở đó. Đi tiếp các panel còn lại.

### 2. Đây là vấn đề của app, path network, hay kernel state-table?

Map nhanh:

- `CLOSE_WAIT` cao:
  - thường nghi app local chưa close socket
- `Retrans` cao:
  - thường nghi path quality / packet loss / congestion / NIC
- `Conntrack%` cao hoặc `Drops > 0`:
  - thường nghi kernel state-table pressure
- `TIME_WAIT` cao:
  - thường nghi short-lived connection churn

### 3. State này là góc nhìn của local host hay mình đang nghĩ theo peer?

Nhắc lại cho bản thân:

- `CLOSE_WAIT` = peer đã đóng trước, local app chưa close
- `TIME_WAIT` = local host đang giữ state cleanup sau close
- đừng nhìn state rồi vô thức suy luận theo “ý muốn” của peer

### 4. Queue đang nói app chậm hay network chậm?

Nhìn `Top Connections`:

- `Send-Q` cao:
  - nghi data chưa gửi thoát được
  - nếu đi cùng retrans cao, nghi path xấu
  - nếu retrans thấp, nghi downstream/peer chậm
- `Recv-Q` cao:
  - nghi app local đọc chậm

### 5. Retrans có đủ sample để tin chưa?

Nếu `Retrans` đang `LOW SAMPLE`:

- chưa nên chốt “network issue”
- cần thêm sample hoặc traffic đủ lớn hơn

Chỉ coi retrans là strong signal khi:

- không còn `LOW SAMPLE`
- và tăng liên tục

### 6. Conntrack đang là symptom phụ hay bottleneck thật?

Nhìn phần Conntrack trong panel `System Health`:

- `Used / Max`
- `Conntrack%`
- `Drops` nếu khác `0`

Kết luận nhanh:

- `%` cao nhưng chưa có `Drops`:
  - đang tiến gần pressure
- có `Drops`:
  - đây là hard-failure signal, ưu tiên xử lý

### 7. Một peer/service cụ thể có đang dominate không?

Đổi `View=GROUP` rồi nhìn:

- `CONNS`
- `PORTS / RPORTS`
- `Selected Detail -> States`

Dùng `Selected Detail` để hiểu:

- row đang chọn đại diện cho cái gì
- nếu đang ở `IN` thì `Enter` / `k` sẽ target `peer + local port` nào, còn `OUT` thì remote service port nào đang dominate

## Nếu thấy pattern này, nghĩ ngay điều này

### `TIME_WAIT` cao

Nghĩ ngay:

- kết nối ngắn hạn đang churn mạnh

Check tiếp:

- client có reuse connection không?
- có health check/burst traffic không?
- đang tập trung vào port nào?

### `CLOSE_WAIT` cao

Nghĩ ngay:

- app local có thể chưa close socket

Check tiếp:

- process nào đang giữ nhiều row?
- app có bug ở cleanup path không?

### `SYN_RECV` cao

Nghĩ ngay:

- handshake đang incomplete

Check tiếp:

- backlog pressure?
- SYN flood?
- client connect nhưng không complete?

### `Retrans` cao

Nghĩ ngay:

- path network / loss / congestion / NIC issue

Check tiếp:

- `ip -s link show dev <iface>`
- `ss -tin`
- `Send-Q` có cao không?

### `Conntrack%` cao hoặc `Drops > 0`

Nghĩ ngay:

- kernel state-table đang bị ép

Check tiếp:

- flow churn
- NAT/firewall state
- `nf_conntrack_max`

## 5 lệnh đối chiếu nhanh

```bash
ss -tan
ss -tanp
ss -tin
conntrack -S
ip -s link show dev eth0
```

## 3 lỗi tư duy hay gặp

### 1. Thấy `TIME_WAIT` cao rồi kết luận app leak

Không đúng theo default.

`TIME_WAIT` thường gần với:

- churn
- short-lived connections

### 2. Thấy retrans cao rồi kết luận app bug

Không đủ.

Retrans thường là path-quality signal trước.

### 3. Thấy Conntrack cao rồi nghĩ app đang giữ quá nhiều socket

Không nhất thiết.

Conntrack là kernel state tracking, không đồng nhất với socket ownership của app.

## Khi bị ngợp, chỉ hỏi 5 câu này

1. Vấn đề nổi bật nhất là state, path, hay state-table pressure?
2. State đó là góc nhìn của local host hay peer?
3. Queue đang nói app local chậm hay network path chậm?
4. Retrans có đủ sample để tin chưa?
5. Có peer/service nào đang dominate không?

Nếu trả lời được 5 câu này, bạn thường đã đủ để quyết định bước điều tra tiếp theo.
