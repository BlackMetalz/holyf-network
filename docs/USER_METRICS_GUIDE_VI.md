# User Metrics Guide (VI)

Tài liệu này giải thích nhanh cách đọc số trong `holyf-network` theo góc nhìn vận hành thực tế.

Nếu bạn còn mới với TCP state / queue / conntrack, nên đọc trước:

- `docs/NETWORK_FOUNDATIONS_FOR_SRE_VI.md`

## Đọc nhanh 30 giây khi có alert

1. Nhìn `Connection States`:
   - `HEALTH` có đỏ/vàng không.
   - `Retrans`, `Drops`, `Conntrack%` có tăng bất thường không.
2. Nhìn `Interface Stats`:
   - `RX/TX` có tăng mạnh không.
   - `Errors/Drops` có khác `0` không.
3. Qua `Top Connections`:
   - Sort theo băng thông (`Shift+B`) để thấy flow nặng nhất.
   - Dò `Send-Q/Recv-Q` bất thường.
4. Nếu có Docker/NAT:
   - Thấy `ct/nat` là bình thường (flow thật, nhưng ownership theo conntrack/NAT).
5. Cần forensic:
   - vào `replay` để xem theo timeline, không kill/block.

## Giải nghĩa theo từng panel

## 1) Top Connections

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
  - `PORTS`: các local port hiện có trong group đó.

`View=CONN` vs `View=GROUP`:

- `CONN`: xem từng connection, tốt để debug cụ thể từng flow.
- `GROUP`: gom theo `(peer, process)` để thấy ai đang chiếm nhiều connection/bandwidth.
  - Ví dụ cùng peer nhưng `sshd` và `ct/nat` sẽ là 2 dòng tách biệt.

Diễn giải nhanh:

- `Send-Q` cao kéo dài + `TX/s` thấp: app gửi ra chậm/tắc downstream.
- `Recv-Q` cao kéo dài: app đọc chậm, queue đang backlog phía receive.
- `TX/s`,`RX/s` đều `0B/s`: có thể flow idle hoặc chưa đủ baseline mẫu đầu.
- `Diagnosis`: câu tóm tắt rule-based cho trạng thái nổi bật nhất của host ở live mode.
  - Đây là summary mức host, không bị thu hẹp theo filter/search hiện tại trong v1.
- `Selected Detail`: phần preview ở cuối panel live giải thích row đang chọn và nếu bấm `Enter` / `k` thì app sẽ target gì.
  - Ở `GROUP`, phần này đặc biệt hữu ích vì action thực tế vẫn resolve về một `peer + local port` cụ thể.
  - Full state breakdown của group cũng nằm ở đây (`States: EST ... - TW ... - CW ...`), không còn nằm trên row list nữa.

## 2) Connection States

- Thể hiện phân bố số connection theo state.
- `Retrans` dùng để nhìn chất lượng đường truyền TCP.
- Nếu hiện `LOW SAMPLE`: mẫu chưa đủ lớn để kết luận retrans tin cậy.

Khi nào đáng lo:

- `Retrans` cao liên tục + `out seg/s` đủ lớn.
- `TIME_WAIT` tăng mạnh kèm churn cao (thường do kết nối ngắn hạn dày).

## 3) Interface Stats

- `RX/TX`: tổng lưu lượng theo NIC (bytes/s).
- `Packets`: số packet/s RX/TX.
- `Errors`, `Drops`: lỗi và drop của interface.

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

Mốc vận hành thường dùng:

- `Conntrack% > 70%`: bắt đầu cảnh giác.
- `Conntrack% > 85%`: mức nguy hiểm, cần xử lý ngay.

## Live vs Replay

- `Live`:
  - dữ liệu realtime hiện tại.
  - có thao tác block/kill (khi chạy quyền phù hợp).
- `Replay`:
  - read-only theo snapshot timeline.
  - mặc định `holyf-network replay` sẽ lấy snapshot của **ngày hiện tại** theo giờ local server.
  - dùng `-f`, `-b`, `-e` để bó hẹp phạm vi.
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

1. Giảm interval xuống `5-10s` nếu đang theo dõi bandwidth.
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
