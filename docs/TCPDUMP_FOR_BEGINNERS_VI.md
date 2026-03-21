# Tcpdump Cho Người Mất Gốc (VI)

Tài liệu này dành cho người mới bắt đầu, mục tiêu là:

1. Biết khi nào nên dùng `tcpdump`.
2. Chạy được capture an toàn, ngắn gọn, không phá máy.
3. Đọc được output cơ bản để trả lời: có traffic không, có timeout/retrans/RST không, có drop packet khi capture không.

## 1) `tcpdump` là gì và dùng khi nào?

- `tcpdump` là công cụ bắt gói tin (packet capture) ở mức mạng.
- Dùng khi metric tổng quát (bandwidth, conntrack, states) báo bất thường nhưng chưa biết gốc do:
  - app không trả lời,
  - mạng chập chờn,
  - reset kết nối,
  - handshake TCP lỗi.

Các câu hỏi `tcpdump` trả lời tốt:

- Có gói tin đi/đến thật không?
- Có SYN đi mà không thấy SYN-ACK về không?
- Có nhiều gói RST không?
- Capture có bị mất gói do quá tải không (`dropped by kernel`)?

## 2) Điều kiện chạy (quyền)

- Bắt packet live thường cần quyền đặc biệt (`root` hoặc capability phù hợp).
- Đọc file capture (`-r`) thì không cần quyền đặc biệt.

Ví dụ kiểm tra nhanh:

```bash
sudo tcpdump -D
```

Nếu lệnh trên liệt kê interface thì máy đã sẵn sàng để capture.

## 3) Bài tập nhập môn: 5 lệnh đủ dùng

### Bước 1: liệt kê interface

```bash
sudo tcpdump -D
```

### Bước 2: nhìn nhanh traffic realtime (không resolve DNS)

```bash
sudo tcpdump -i eth0 -nn -c 50
```

- `-i eth0`: chọn interface.
- `-nn`: không đổi IP/port sang tên (đỡ chậm, dễ grep).
- `-c 50`: bắt 50 packet rồi tự dừng.

### Bước 3: lọc theo host/port để giảm nhiễu

```bash
sudo tcpdump -i eth0 -nn 'host 10.10.10.25 and port 443' -c 200
```

### Bước 4: lưu pcap để phân tích sau

```bash
sudo tcpdump -i eth0 -nn -s 0 -w /tmp/case-443.pcap 'host 10.10.10.25 and port 443'
```

- `-s 0`: lấy full packet (không truncate payload).
- `-w`: ghi file pcap.

### Bước 5: đọc lại file

```bash
tcpdump -nn -r /tmp/case-443.pcap
```

## 4) Filter căn bản (quan trọng nhất)

Capture filter dùng cú pháp `pcap-filter` (BPF). Nên bắt đầu từ các primitive sau:

- `host 1.2.3.4`
- `src host 1.2.3.4`
- `dst host 1.2.3.4`
- `port 443`
- `portrange 30000-30100`
- `tcp`, `udp`, `icmp`
- ghép với `and`, `or`, `not`, ngoặc `()`

Ví dụ:

```bash
# Chỉ TCP 443 tới/đi từ 10.10.10.25
tcp and host 10.10.10.25 and port 443

# Chỉ DNS
port 53

# Bỏ ARP và DNS
not arp and not port 53
```

## 5) Bộ lệnh thực chiến cho incident

### 5.1 Theo dõi timeout TCP (SYN đi, SYN-ACK về ít)

```bash
sudo tcpdump -i eth0 -nn 'tcp[tcpflags] & (tcp-syn) != 0 and tcp[tcpflags] & (tcp-ack) = 0'
```

### 5.2 Theo dõi reset kết nối (RST)

```bash
sudo tcpdump -i eth0 -nn 'tcp[tcpflags] & tcp-rst != 0'
```

### 5.3 Chỉ bắt chiều IN hoặc OUT (nếu platform hỗ trợ)

```bash
sudo tcpdump -i eth0 -nn -Q in  'tcp and port 443'
sudo tcpdump -i eth0 -nn -Q out 'tcp and port 443'
```

## 6) Cách đọc output nhanh trong 30 giây

Nhìn theo thứ tự:

1. Timestamp có đều không.
2. 5-tuple có đúng target không (src/dst ip:port).
3. TCP flags:
   - `[S]`: SYN
   - `[S.]`: SYN-ACK
   - `[.]`: ACK
   - `[P.]`: push + ack (thường có data)
   - `[R]` hoặc có `R`: reset
4. Khi kết thúc capture, đọc 3 con số tổng kết:
   - `packets captured`
   - `received by filter`
   - `dropped by kernel` (quan trọng để biết capture có bị thiếu không)

## 7) Mẹo tránh tự bắn vào chân

1. Luôn lọc hẹp (host/port/protocol), đừng capture toàn mạng quá lâu.
2. Luôn giới hạn thời gian hoặc số gói:
   - số gói: `-c`
   - thời gian: `timeout 10s tcpdump ...`
3. Nếu thấy `dropped by kernel` tăng:
   - lọc hẹp hơn,
   - giảm verbosity,
   - tăng buffer (`-B`).
4. Dùng `-w` để lưu file, phân tích sau bằng `tcpdump -r` hoặc Wireshark.
5. Cẩn thận dữ liệu nhạy cảm trong pcap (token/cookie/payload).

## 8) Mapping nhanh với `holyf-network`

Khi `Top Connections` có peer/port nghi ngờ:

1. Lấy `peer_ip` + `port` từ row đang chọn.
2. Capture ngắn 5-15 giây:

```bash
sudo timeout 15s tcpdump -i eth0 -nn -s 0 -w /tmp/peer-check.pcap \
  'host <peer_ip> and port <port>'
```

3. Đọc nhanh:

```bash
tcpdump -nn -r /tmp/peer-check.pcap | head -n 200
```

Mục tiêu: xác nhận bất thường là do path/network hay do app behavior.

## 9) Nguồn tham khảo chính thống

- `tcpdump(8)` Linux man page (option, output, capture counters):  
  https://www.man7.org/linux/man-pages/man8/tcpdump.8.html
- `pcap-filter(7)` reference (cú pháp filter BPF):  
  https://www.wireshark.org/docs/man-pages/pcap-filter.html
- Wireshark User Guide - capture filter overview (cú pháp và nguyên tắc):  
  https://www.wireshark.org/docs/wsug_html_chunked/ChCapCaptureFilterSection.html
- `pcap(3pcap)` (quyền capture, promiscuous mode, buffer behavior):  
  https://man7.org/linux/man-pages/man3/pcap.3pcap.html
- Wireshark CaptureFilters wiki (nhiều ví dụ filter thực tế):  
  https://wiki.wireshark.org/CaptureFilters

Ghi chú:

- Bản tham chiếu chuẩn của `pcap-filter(7)` thuộc dự án tcpdump/libpcap; mirror trên Wireshark docs dễ truy cập hơn trong nhiều môi trường.
