# TCPDump Trace Feature (VI)

Tài liệu này mô tả feature `T (Trace packet)` tích hợp trong `holyf-network` live TUI.

## 1) Mục tiêu feature

- Cho operator trace nhanh packet của peer đang chọn ngay trong `Top Connections`.
- Không phải gõ tay `tcpdump` command trong lúc incident.
- Giữ capture có giới hạn rõ ràng để tránh chạy trôi.

## 2) UX flow

### Bước 1: Mở form cấu hình

- Focus vào panel `Top Connections`.
- Chọn row cần trace.
- Bấm `T`.
- App mở popup `Trace Packet` (form cấu hình).

Field trong form:

- `Peer IP`: prefill theo row đang chọn.
- `Port`: prefill theo row đang chọn.
  - `IN`: lấy local service port.
  - `OUT`: lấy remote service port.
- `Interface`: prefill từ interface hiện tại.
- `Scope`:
  - `Peer + Port` (mặc định, khuyến nghị)
  - `Peer only`
- `Direction`: `ANY` / `IN` / `OUT` (prefill theo mode `IN/OUT` hiện tại).
- `Duration (s)`: mặc định `10`, max `60`.
- `Packet cap`: mặc định `2000`, max `20000`.
- `Save pcap`: mặc định bật.

### Bước 2: Chạy capture và theo dõi tiến trình

- Bấm `Start` để chạy.
- App mở popup `Trace Packet Progress`:
  - hiện countdown theo giây,
  - hiện interface, scope, direction, duration, cap.
- Có thể abort bằng `Esc` hoặc `q`.

### Bước 3: Xem kết quả

- Khi capture xong, app mở popup `Trace Packet Result`:
  - filter đã chạy,
  - số packet captured / received-by-filter / dropped-by-kernel,
  - số `SYN`, `SYN-ACK`, `RST`,
  - khối `Trace Analyzer` với `Severity`, `Confidence`, `Issue`, `Signal`, `Likely`, `Check next`,
  - sample packet lines decode từ pcap,
  - trạng thái `completed` / `aborted` / `failed`.
- Đóng popup bằng `Enter` hoặc `Esc`.

## 3) Guardrails kỹ thuật

- Chỉ chạy nếu có `tcpdump` trên host.
- Filter luôn bị giới hạn vào `tcp and host <peer>`; nếu chọn `Peer + Port` thì thêm `and port <port>`.
- Capture luôn bounded bởi cả:
  - timeout (`Duration`),
  - packet cap (`-c`).
- Dùng `-s 128` (snaplen ngắn) để giảm overhead.
- Nếu `Save pcap` tắt: file tạm bị xóa sau khi đọc summary.
- Nếu `Save pcap` bật: lưu ở `/tmp/holyf-network-captures`.

## 4) Command tương đương (để đối chiếu)

App sinh command tương đương ý nghĩa như:

```bash
tcpdump -i <iface> -nn -tt -s 128 -c <packet_cap> [-Q in|out] -w <pcap_path> "tcp and host <peer> [and port <port>]"
```

Sau đó app đọc lại pcap:

```bash
tcpdump -nn -tt -r <pcap_path>
```

## 5) Trace Analyzer (Phase 2)

`Trace Analyzer` là bộ rule-based static analyzer để đưa kết luận nhanh từ sample packet.

- `Severity`:
  - `INFO`: chưa thấy tín hiệu bất thường mạnh trong sample.
  - `WARN`: có tín hiệu nghi ngờ cần điều tra tiếp.
  - `CRIT`: có tín hiệu lỗi rõ hoặc độ rủi ro cao.
- `Confidence`:
  - `LOW`: sample nhỏ.
  - `MEDIUM`: sample vừa.
  - `HIGH`: sample đủ lớn hoặc capture lỗi rõ ràng.

Rule hiện tại (thứ tự ưu tiên):

1. `Capture failed` -> `CRIT`.
2. `DroppedByKernel > 0`:
   - `WARN` mặc định,
   - `CRIT` nếu drop ratio cao hoặc số packet drop lớn.
3. `RST pressure`:
   - cảnh báo khi tỷ lệ `RST` cao.
4. `SYN seen but no SYN-ACK` hoặc `Low SYN-ACK ratio`.
5. `Low packet sample` nếu sample quá ít.
6. Mặc định: `No strong packet-level anomaly`.

Analyzer chỉ hỗ trợ triage nhanh, không thay thế packet forensics đầy đủ.

## 6) Troubleshooting

### Lỗi va chạm path khi cài binary

Triệu chứng:

```text
mv: cannot overwrite non-directory '/usr/local/bin/holyf-network' with directory '/tmp/holyf-network'
```

Nguyên nhân: host đang có thư mục cũ `/tmp/holyf-network`, trùng tên file tạm dùng để tải binary.

Cách xử lý:

```bash
sudo rm -rf /tmp/holyf-network
curl -sL https://github.com/BlackMetalz/holyf-network/releases/latest/download/holyf-network-linux-amd64 -o /tmp/holyf-network.bin
chmod +x /tmp/holyf-network.bin
sudo mv /tmp/holyf-network.bin /usr/local/bin/holyf-network
sudo holyf-network -v
```

Lưu ý: từ bản mới, pcap mặc định lưu ở `/tmp/holyf-network-captures` để tránh đụng path cài đặt.

## 7) Lưu ý vận hành

- Capture trong app phục vụ triage nhanh, không thay thế full packet-forensics dài hạn.
- Nếu cần forensic sâu:
  - tăng thời lượng ngoài app theo runbook riêng,
  - đồng bộ với change window và guardrail của team.

## 8) Tài liệu tham khảo chính thống

- `tcpdump` man page (tcpdump.org): https://www.tcpdump.org/manpages/tcpdump.1.html
- `pcap-filter` syntax (tcpdump/libpcap): https://www.tcpdump.org/manpages/pcap-filter.7.html
- Linux man-pages mirror (`pcap-filter(7)`): https://man7.org/linux/man-pages/man7/pcap-filter.7.html
- Linux man-pages mirror (`tcpdump(8)`): https://man7.org/linux/man-pages/man8/tcpdump.8.html
