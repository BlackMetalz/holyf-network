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
- `Mode` (control chính):
  - `General triage` (mặc định)
  - `Handshake`
  - `Packet loss`
  - `Reset storm`
  - `Custom` (manual mode).
- `Filter strategy`:
  - chỉ hiện khi `Mode=Custom`,
  - gồm `Peer + Port`, `Peer only`, `5-tuple`, `SYN/RST only`.
- `Extra BPF clause`:
  - chỉ hiện khi `Mode=Custom`,
  - app sẽ ghép theo dạng: `(strategy filter) and (<extra clause>)`.
- `Direction`: `ANY` / `IN` / `OUT` (prefill theo mode `IN/OUT` hiện tại).
- `Duration (s)`: mặc định `10`, max `60`.
- `Packet cap`: mặc định `2000`, max `20000`.
- `Save pcap`: mặc định bật.
- `Enter` trong form sẽ chạy `Start` ngay (không cần tab tới nút `Start`).
- Form luôn hiển thị `Mode / Strategy Guide` ở phía dưới để xem nhanh context.
- Form luôn hiển thị `Tcpdump Command Preview` realtime theo input hiện tại.
- Form có thêm `Tcpdump Flags Guide` ngay dưới preview để giải thích nhanh `-nn`, `-tt`, `-s 128` (kèm `-c`, `-Q` theo context hiện tại).

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

### Bước 4: Mở Trace History (Phase 3A)

- Bấm `t` trong live TUI để mở `Trace History`.
- Modal hiển thị các lần trace gần nhất:
  - `Up/Down`: chọn run.
  - `Enter`: mở detail (nội dung tương tự `Trace Packet Result` + analyzer).
  - `c`: chọn `baseline` rồi chọn `incident` để mở compare modal.
  - `Esc`: đóng.
- Compare modal hiển thị diff:
  - `drop ratio`,
  - `RST ratio`,
  - `SYN-ACK ratio`,
  - `top flags changed` (incident - baseline).

### Bước 5: Replay timeline thấy trace event theo mốc snapshot (Phase 3B)

- Khi chạy `holyf-network replay`, panel timeline sẽ tự hiển thị block `Trace timeline`.
- App map mỗi trace event vào snapshot gần nhất theo cửa sổ thời gian động (dựa trên khoảng cách snapshot thực tế).
- Ở mỗi snapshot:
  - nếu có trace gần mốc đó: hiện số event + preview (time, severity, category/preset, peer:port, issue),
  - nếu không có: hiện `0 events near this snapshot`.
- Status bar replay có thêm chỉ báo `TRACE <near-current>/<associated>` để biết nhanh snapshot hiện tại có trace hay không.
- Nếu không có `connections-*.jsonl` trong scope/range nhưng có `trace-history-*.jsonl`, replay tự chuyển sang **trace-only fallback**:
  - timeline dựng theo từng trace event (dùng `[` `]` để đi qua event),
  - status bar hiện `TRACE-ONLY`,
  - panel hiển thị `Trace-only replay mode`.
- Replay có thêm mode toggle:
  - `g`: chuyển `CONN <-> TRACE` view.
  - `TRACE` view tập trung vào trace events tại mốc hiện tại.
- Replay có thêm nút xem full history:
  - `h`: mở modal `Replay Trace History` (list + detail).
  - trong modal này cũng hỗ trợ `c` để compare 2 trace.

## 3) Guardrails kỹ thuật

- Chỉ chạy nếu có `tcpdump` trên host.
- Filter luôn bị giới hạn vào `tcp and host <peer>` (base).
- Strategy sẽ mở rộng base:
  - `Peer + Port`: thêm `and port <port>`.
  - `Peer only`: giữ base.
  - `5-tuple`: sinh filter tuple hai chiều cho 1 flow cụ thể.
  - `SYN/RST only`: thêm điều kiện tcp flags SYN/RST.
- `Mode` là lớp UX/runbook:
  - `General/Handshake/Packet loss/Reset storm` tự gán strategy + direction + duration + cap mặc định,
  - `Custom` cho phép chọn strategy thủ công + thêm extra BPF clause.
- Capture luôn bounded bởi cả:
  - timeout (`Duration`),
  - packet cap (`-c`).
- Dùng `-s 128` (snaplen ngắn) để giảm overhead.
- Nếu `Save pcap` tắt: file tạm bị xóa sau khi đọc summary.
- Nếu `Save pcap` bật: lưu ở `/tmp/holyf-network-captures`.
  - Tên file có category theo preset để dễ nhìn nhanh:
  - `trace-YYYYMMDD-HHMMSS-<preset>-<peer>-<port>.pcap`
  - Ví dụ: `trace-20260322-103011-syn-rst-14_231_106_188-22.pcap`

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

## 7) Trace History persistence (Phase 3A)

- Mỗi lần trace xong (`completed` / `completed-timeout` / `aborted` / `failed`), app ghi 1 event vào **cùng data-dir với replay/snapshot** (`history.DefaultDataDir()`).
- Tên file theo ngày (server local time):
  - `trace-history-YYYYMMDD.jsonl`
- Event lưu các trường chính:
  - context (`peer`, `port`, `interface`, `category/preset`, `scope`, `direction`, `filter`)
  - counters (`captured`, `received-by-filter`, `dropped-by-kernel`, `SYN`, `SYN-ACK`, `RST`)
  - analyzer (`severity`, `confidence`, `issue`, `signal`, `likely`, `check next`)
  - trạng thái run + lỗi capture/read (nếu có) + sample packet rút gọn.
- Retention dùng cùng policy với replay history (mặc định `168h`).

## 8) Replay timeline integration (Phase 3B)

- Replay đọc trace history từ cùng `data-dir` đang dùng để load snapshot replay.
- Mapping là nearest-snapshot (không sửa file snapshot cũ).
- Dữ liệu cũ chưa có field `preset` vẫn hiển thị category nhờ fallback từ `scope`.
- Replay hotkeys được mở rộng:
  - `g` toggle view `CONN/TRACE`,
  - `h` mở replay trace history modal.
- Khi chạy ở trace-only fallback, `Shift+S` (timeline snapshot search) bị disable vì không có snapshot record để scan.
## 9) Lưu ý vận hành

- Capture trong app phục vụ triage nhanh, không thay thế full packet-forensics dài hạn.
- Nếu cần forensic sâu:
  - tăng thời lượng ngoài app theo runbook riêng,
  - đồng bộ với change window và guardrail của team.

## 10) Tài liệu tham khảo chính thống

- `tcpdump` man page (tcpdump.org): https://www.tcpdump.org/manpages/tcpdump.1.html
- `pcap-filter` syntax (tcpdump/libpcap): https://www.tcpdump.org/manpages/pcap-filter.7.html
- Linux man-pages mirror (`pcap-filter(7)`): https://man7.org/linux/man-pages/man7/pcap-filter.7.html
- Linux man-pages mirror (`tcpdump(8)`): https://man7.org/linux/man-pages/man8/tcpdump.8.html
