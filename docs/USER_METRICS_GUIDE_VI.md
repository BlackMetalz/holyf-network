# User Metrics Guide (VI)

Tài liệu này giải thích nhanh cách đọc số trong `holyf-network` theo góc nhìn vận hành thực tế.

Nếu bạn còn mới với TCP state / queue / conntrack, nên đọc trước:

- `docs/NETWORK_FOUNDATIONS_FOR_SRE_VI.md`

## Đọc nhanh 30 giây khi có alert

1. Nhìn `Connection States`:
   - `HEALTH` có đỏ/vàng không.
   - `Retrans` hoặc `Drops` có tăng bất thường không.
2. Nhìn `Interface Stats`:
   - `RX/TX` có tăng mạnh không.
   - `Errors/Drops` có khác `0` không.
   - Nếu dòng `Traffic` xuất hiện thì coi đó là cảnh báo spike ngắn ở interface.
3. Qua `Top Connections`:
   - Sort theo băng thông (`Shift+B`) để thấy flow nặng nhất.
   - Dò `Send-Q/Recv-Q` bất thường.
4. Nếu có Docker/NAT:
   - Thấy `ct/nat` là bình thường (flow thật, nhưng ownership theo conntrack/NAT).
5. Cần forensic:
   - vào `replay` để xem theo timeline, không kill/block.

## Giải nghĩa theo từng panel

## 1) Top Connections

Panel 1 ở live có thể chuyển giữa:

- `Dir=IN` (`Top Incoming`): flow đi vào local service đang listen.
- `Dir=OUT` (`Top Outgoing`): flow do process local chủ động đi ra ngoài.
  - Bấm `o` để toggle.
  - `Enter` / `k` chỉ bật ở `IN`.
  - Bấm `T` để mở flow `Trace Packet` (tcpdump bounded) theo row đang chọn.
    - Chi tiết UX/guardrails: `docs/TCPDUMP_TRACE_FEATURE_VI.md`.
  - Dùng `[` / `]` để qua trang trước/sau khi danh sách dài hơn chiều cao panel (áp dụng cho cả `IN/OUT` và `CONN/GROUP`).
  - Footer hiển thị ngữ cảnh trang hiện tại (ví dụ `Showing 16-30 of 42 connections | Page 2/3`).

Các cột chính:

- `PROCESS`: tiến trình sở hữu socket.
  - `PID/NAME` (vd `44011/sshd`): map được host process.
  - `ct/nat`: flow suy ra từ conntrack/NAT, không map thẳng host PID.
  - `-`: chưa map được process.
- `SRC`, `PEER`: đầu local và đầu remote.
- `STATE`: TCP state (`ESTABLISHED`, `TIME_WAIT`, ...).
- `Send-Q`, `Recv-Q`: backlog queue snapshot ở thời điểm lấy mẫu.
- `TX/s`, `RX/s`: throughput theo byte delta conntrack trong chu kỳ refresh hiện tại.
- Trong `View=GROUP`, mỗi dòng còn tóm tắt thêm:
  - `PORTS`: các local port của group khi đang ở `IN`.
  - `RPORTS`: các remote service port của group khi đang ở `OUT`.

`View=CONN` vs `View=GROUP`:

- `CONN`: xem từng connection, tốt để debug cụ thể từng flow.
- `GROUP`: gom theo `(peer, process)` để thấy ai đang chiếm nhiều connection/bandwidth.
  - Ví dụ cùng peer nhưng `sshd` và `ct/nat` sẽ là 2 dòng tách biệt.
  - `GROUP` ở live bị cap còn top `20` group theo `CONNS` để panel vẫn dễ đọc.
- `IN` vs `OUT`:
  - `IN`: port filter và `PORTS` trong group là local service port.
  - `OUT`: port filter và `RPORTS` trong group là remote destination port; panel chỉ để quan sát, không kill/block.

Diễn giải nhanh:

- `Send-Q` cao kéo dài + `TX/s` thấp: app gửi ra chậm/tắc downstream.
- `Recv-Q` cao kéo dài: app đọc chậm, queue đang backlog phía receive.
- `TX/s`,`RX/s` đều `0B/s`: có thể flow idle hoặc chưa đủ baseline mẫu đầu.
- `Selected Detail`: phần preview ở cuối panel live giải thích row đang chọn và nếu bấm `Enter` / `k` thì app sẽ target gì.
  - Ở `GROUP`, phần này đặc biệt hữu ích vì action thực tế vẫn resolve về một `peer + local port` cụ thể.
  - Full state breakdown của group cũng nằm ở đây (`States: EST ... - TW ... - CW ...`), không còn nằm trên row list nữa.
- Ở `OUT`, `Selected Detail` sẽ chuyển sang ngữ cảnh remote port và nói rõ `Enter` / `k` đang bị tắt.
- Panel Top live sẽ ẩn TCP connection do chính process `holyf-network` hiện tại tạo ra, để traffic control-plane như update check không chui vào danh sách operator đang xem.

## 2) Connection States

- Thể hiện phân bố số connection theo state.
- `Retrans` dùng để nhìn chất lượng đường truyền TCP.
- Nếu hiện `LOW SAMPLE`: mẫu chưa đủ lớn để kết luận retrans tin cậy.

Khi nào đáng lo:

- `Retrans` cao liên tục + `out seg/s` đủ lớn.
- `TIME_WAIT` tăng mạnh kèm churn cao (thường do kết nối ngắn hạn dày).

## 3) Interface Stats

- `RX/TX`: tổng lưu lượng theo NIC (bytes/s).
- `Packet rate`: số packet/s RX/TX.
- `App`: số core CPU đang dùng và RSS memory của process holyf-network hiện tại, không phải số host-wide của cả máy.
- `Traffic`: cảnh báo spike ngắn, chỉ hiện khi traffic cần chú ý.
- `Errors`, `Drops`: lỗi và drop của interface.
- Ở live mode, panel này refresh mỗi `1s` để thấy spike bandwidth nhanh hơn.
- Nhịp `1s` này chỉ áp dụng cho throughput/packet của NIC.
- Dòng `App` (CPU/RSS) lấy mẫu theo refresh interval cấu hình (`-r/--refresh`).
- Ngay sau lúc startup có một warm-up refresh sớm (~1s) để ổn định sample đầu.
- App CPU cần 2 mẫu theo refresh interval để ra giá trị core ổn định.

Ý nghĩa dòng `Traffic`:

- Mặc định bị ẩn khi interface đang yên / ổn định.
- Chỉ hiện khi có spike mức warn/crit.
- Ưu tiên dùng speed NIC nếu đọc được; nếu không thì fallback về throughput + tỷ lệ so với baseline di động.
- Footer có debug link-speed: `LINK(sysfs):<Mb/s>` hoặc `LINK(sysfs):UNKNOWN`.

Cách đối chiếu:

- Interface cao nhưng Top thấp: traffic có thể rất ngắn hạn, flow không giữ lâu trong sample.
- Interface thấp nhưng một flow Top rất cao: có thể vài flow lớn đang chi phối.

## 4) Conntrack

Đây là mức dùng bảng state tracking của kernel (`nf_conntrack`), không phải “số port outbound mở”.

- `Used / Max`: số entry đang track trên tổng capacity.
- `Drops`: số flow không insert được (thường do bảng đầy hoặc lỗi).

Panel live ưu tiên nhìn áp lực bảng state:

- tập trung vào `Used / Max` và `Conntrack%`
- chỉ cần đặc biệt chú ý `Drops` nếu khác `0`
  - `Drops: 0` được ẩn đi chủ đích để panel đỡ nhiễu

Ngưỡng Conntrack lấy từ `config/health_thresholds.toml` (hoặc dùng default built-in nếu không có file).
Mặc định conntrack là `warn >= 70%` và `crit >= 85%`.

## 5) Diagnosis

- Là panel live riêng, gồm:
  - `Issue`: vấn đề host-level nổi bật nhất ở thời điểm hiện tại
  - `Scope`: peer/port nổi trội nếu xác định được, nếu không thì là `host-wide`
  - `Signal`: một dòng metric ngắn nối diagnosis với TCP states / retrans / conntrack
  - `Likely Cause`: nguyên nhân nghi ngờ chính (diễn giải ngắn)
  - `Confidence`: mức tin cậy `LOW` / `MEDIUM` / `HIGH`
  - `Why`: tóm tắt bằng chứng chính cho kết luận
  - `Next Actions`: tối đa 3 bước nên kiểm tra ngay
- Trong v1, panel này vẫn là host-global nên không bị bó hẹp theo filter/search/group selection ở `Top Connections`.
- Mục đích là nối nhanh tín hiệu giữa các panel, không thay thế việc đọc panel gốc.
- Bấm `d` để mở `Diagnosis History`, là modal lưu các lần diagnosis đổi trong live session hiện tại.

## Live vs Replay

- `Live`:
  - dữ liệu realtime hiện tại.
  - có thao tác block/kill (khi chạy quyền phù hợp).
- `Replay`:
  - read-only theo snapshot timeline.
  - mặc định `holyf-network replay` sẽ lấy snapshot của **ngày hiện tại** theo giờ local server.
  - dùng `-f`, `-b`, `-e` để bó hẹp phạm vi.
  - bấm `o` để toggle replay giữa aggregate `IN` và aggregate `OUT`.
  - bấm `g` để toggle view `CONN <-> TRACE`.
  - bấm `h` để mở `Replay Trace History` (list/detail các trace run).
  - replay ở v1 vẫn chỉ có aggregate view, không có raw conn view.
  - khác với `GROUP` ở live, replay không áp business cap top-20; nó hiển thị toàn bộ row đã lưu cho direction đang chọn.
  - không render `Diagnosis` ở phase hiện tại.

## Block vs Kill

- `minutes > 0`:
  - app chèn rule block trước
  - quét và kill lại toàn bộ connection khớp `peer + local port`
  - giữ block tới khi hết hạn
- `minutes = 0`:
  - app chỉ chạy kill sweep, không chèn block rule
  - nếu storm đang diễn ra thì có thể vẫn xuất hiện connection mới trong lúc sweep
- `TIME_WAIT` không bị tính là kill fail. Nếu còn `remaining N (storm/race)` thì nghĩa là vẫn còn active flow sau giới hạn sweep.

## Troubleshooting Playbook

## Case 1: Interface có traffic nhưng Top không thấy rõ

Check nhanh:

```bash
sudo ss -ntp
sudo conntrack -L -p tcp | head -n 30
sudo sysctl net.netfilter.nf_conntrack_acct
```

Gợi ý xử lý:

1. Giảm interval xuống `5-10s` nếu đang theo dõi bandwidth ở **Top Connections**.
2. Bấm `r` để lấy thêm mẫu baseline.
3. Dùng `f` (lọc port) và `/` (search text) để thu hẹp context.

## Case 2: Docker/MySQL không hiện process thật

Hiện `ct/nat` là expected cho flow NAT host-facing.

```bash
sudo conntrack -L -p tcp | grep -E 'dport=3306|sport=3306'
docker ps
# tùy nhu cầu debug sâu netns:
sudo nsenter -t <container_pid> -n ss -ntp | grep ':3306'
```

## Case 3: `TX/s`/`RX/s` bằng 0

Khả năng thường gặp:

1. Sample đầu chưa có baseline delta.
2. Flow quá ngắn, không kịp xuất hiện trong cửa sổ lấy mẫu.
3. Thiếu quyền hoặc conntrack accounting chưa đúng trên host.

## Case 4: Kill báo `remaining N (storm/race)`

Ý nghĩa:

1. App đã chạy kill sweep lặp (`ss -K` + `conntrack -D`) nhưng dừng theo giới hạn thời gian/vòng lặp.
2. Đây thường là race trong conn storm (flow mới xuất hiện liên tục).
3. `TIME_WAIT` không bị tính là kill-fail; app chỉ coi còn active state là chưa sạch.

Gợi ý xử lý:

1. Nếu cần chặn mạnh hơn, dùng block có thời hạn (`minutes > 0`) thay vì kill-only.
2. Kết hợp filter theo port và theo dõi thêm vài chu kỳ refresh để xác nhận trend giảm.

## Cheat-sheet hành động nhanh

| Triệu chứng | Ý nghĩa vận hành | Hành động ngay |
|---|---|---|
| `Conntrack%` cao liên tục | Bảng state gần đầy | Kiểm tra `nf_conntrack_max`, giảm churn, điều tra nguồn tạo flow |
| `Drops` trong Conntrack > 0 | Kernel không insert được flow | Ưu tiên xử lý capacity/churn trước, kiểm tra rule firewall/NAT |
| `ct/nat` chiếm đa số | Traffic chủ yếu đi qua NAT/container | Dùng filter theo port + conntrack để khoanh vùng service |
| `Send-Q` cao kéo dài | App gửi ra bị nghẽn | Kiểm tra downstream/latency/remote receive |
| `Recv-Q` cao kéo dài | App xử lý đọc vào chậm | Kiểm tra CPU/app bottleneck/read loop |
| `Retrans` cao và không còn LOW SAMPLE | Chất lượng TCP xấu | Soi packet loss/RTT/path, đối chiếu Interface errors/drops |
