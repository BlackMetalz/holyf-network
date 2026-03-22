# Alert Threshold Profiles (VI)

Tài liệu này mô tả cơ chế ngưỡng cảnh báo theo profile trong live mode.

## Mục tiêu

Mỗi loại workload có "shape" traffic khác nhau.  
Dùng một bộ ngưỡng cho tất cả host sẽ dễ bị:

- báo động giả (false alert), hoặc
- bỏ sót bất thường thật.

Profile giúp đổi độ nhạy theo vai trò host:

- `WEB`
- `DB`
- `CACHE`

## Cách dùng

- Chọn profile ngay từ lúc chạy:

```bash
sudo holyf-network --alert-profile web
sudo holyf-network --alert-profile db
sudo holyf-network --alert-profile cache
```

- Đang chạy live:
  - bấm `y` để cycle `WEB -> DB -> CACHE`.
  - bấm `Shift+Y` để mở modal giải thích nhanh ngay trong live mode.

## Phạm vi tác động hiện tại (Phase 1)

Ngưỡng profile hiện đang áp dụng cho:

1. `3. Interface Stats` (dòng `Traffic` đánh giá spike)
2. `4. Conntrack` (mốc cảnh báo/nguy hiểm)
3. Nhánh conntrack trong:
   - health strip `Connection States`
   - `5. Diagnosis`

## Bảng ngưỡng profile

Khi đọc được tốc độ NIC, Interface dùng ngưỡng theo `% utilization`:

| Profile | Conntrack Warn/Crit | Interface Util Warn/Crit | Spike Ratio Warn/Crit |
|---|---|---|---|
| WEB | `70% / 85%` | `60% / 85%` | `2.0x / 3.0x` |
| DB | `55% / 70%` | `45% / 70%` | `1.6x / 2.2x` |
| CACHE | `75% / 90%` | `70% / 90%` | `2.5x / 3.8x` |

Nếu NIC không đọc được speed, Interface fallback về ngưỡng peak tuyệt đối:

- `WEB`: `80 / 200 MiB/s`
- `DB`: `40 / 120 MiB/s`
- `CACHE`: `120 / 320 MiB/s`

## Logic đánh giá spike Interface

Panel live dùng đồng thời 2 điều kiện:

1. **Ngưỡng tuyệt đối**
   - Nếu đọc được speed NIC: so `% utilization` theo ngưỡng util của profile.
   - Nếu không đọc được speed: dùng `peak = max(RX_bytes_per_sec, TX_bytes_per_sec)` theo ngưỡng tuyệt đối.
2. **Ngưỡng tương đối**
   - `ratio = peak / max(EWMA_baseline, baseline_floor)`

Mức cảnh báo cuối là mức cao hơn giữa 2 điều kiện.

## Lưu ý

- Startup cần vài sample để warm baseline.
- Khi đổi profile bằng `y`, baseline Interface được reset có chủ đích để tránh sai lệch chéo profile.
- Conntrack rất nhỏ nhưng >0 sẽ hiển thị `<0.1%` thay vì `0%` để dễ đọc hơn.
