  Tình Trạng Triển Khai (2026-03-26)

  - Hoàn thành wave 1: tách `internal/tui/layout`, `internal/tui/overlays`, `internal/tui/shared` thành subpackage thật và rewire live/replay sang API mới.
  - Hoàn thành wave 2: tách thêm `internal/tui/shared/{health,diagnosis,format}` và move renderer `conntrack/diagnosis` sang `internal/tui/panels`.
  - Hoàn thành wave 3: tách `trace history` model + daily segment storage + retention sang `internal/tui/trace`.
  - Hoàn thành wave 4: tách trace list/detail/compare formatting sang `internal/tui/trace/render.go`; live/replay chỉ còn orchestration/UI.
  - Hoàn thành wave 5: tạo `internal/tui/replay` cho replay range filtering, trace-history loading, trace-only fallback refs, và trace timeline association.
  - Hoàn thành wave 6: move thêm renderer `interface` sang `internal/tui/panels`, chuyển test tương ứng sang subpackage `panels`, xóa root duplicate `panel_interface.go`.
  - Hoàn thành wave 7: tách replay search/navigation pure helpers sang `internal/tui/replay/{search,navigation}.go`, rewire `HistoryApp` để gọi package helpers thay vì giữ logic scan/skip-empty trực tiếp ở root.
  - Hoàn thành wave 8: xóa `trace_aliases.go`, cho live/replay/tests import thẳng `internal/tui/trace`, đồng thời fold explain wrapper mỏng vào `app_core.go`/`history_keys.go` và xóa `interface_stats_explain.go`, `socket_queue_explain.go`.
  - Root `internal/tui` hiện còn 41 file top-level, giảm từ khoảng 56 ban đầu. Phần còn lại ở root chủ yếu là coordinator live/replay, diagnosis engine, blocking flow, và transitional bridge `shared_aliases.go`.
  - Ghi chú kỹ thuật: Go không hỗ trợ giữ cùng một package xuyên nhiều thư mục con; vì vậy plan triển khai thực tế dùng subpackage thật + seam rõ ràng, không phải chỉ “move file nhưng giữ package tui”.

  Mục Tiêu

  - giảm vai trò god-object của internal/tui/app_core.go
  - tách rõ live, replay, trace, panels, overlays, shared
  - để mỗi feature có 1 chỗ ở rõ ràng, dễ tìm, dễ test
  - giữ behavior hiện tại, không đổi UX trong lúc refactor

  Vấn Đề Hiện Tại

  - internal/tui có khoảng 56 file top-level
  - live và replay đang nằm chung mặt phẳng
  - trace packet đã thành một subsystem riêng nhưng vẫn lẫn vào flat folder
  - panel render, modal/help, runtime logic, history logic đang trộn nhau
  - test cũng dàn phẳng nên khó nhìn coverage theo module

  Target Structure

  internal/tui/
    live/
      app.go
      refresh.go
      keys.go
      statusbar.go
      top_connections.go
      blocking.go
      diagnosis_history.go

    replay/
      app.go
      keys.go
      layout.go
      timeline_search.go
      trace_history_modal.go
      trace_timeline.go

    trace/
      form.go
      runtime.go
      analyzer.go
      history.go
      compare.go
      model.go

    panels/
      connections.go
      conntrack.go
      diagnosis.go
      interface.go
      top_connections.go
      history_aggregate.go

    overlays/
      help.go
      socket_queue_explain.go
      interface_stats_explain.go
      action_log.go

    shared/
      health.go
      format.go
      conntrack_percent.go
      hotkeys.go
      update_check.go

    layout/
      live.go
      replay.go

  Ownership Rules

  - live/: app state + event routing + refresh orchestration cho live mode
  - replay/: app state + key handling + timeline behavior của replay
  - trace/: toàn bộ capture flow, trace history, compare, analyzer
  - panels/: chỉ render text/panel, tránh cầm app state nặng
  - overlays/: help/modal/explain
  - shared/: formatter, health helpers, reusable UI strings, update check
  - layout/: grid/layout construction thuần

  File Mapping Đề Xuất

  - internal/tui/app_core.go -> live/app.go, live/refresh.go, live/keys.go, live/statusbar.go
  - app_top_connections*.go -> live/top_connections.go + panels/top_connections.go
  - app_blocking_*.go -> live/blocking.go
  - app_diagnosis_history.go -> live/diagnosis_history.go
  - internal/tui/history_app.go, history_keys.go, history_layout.go, history_* -> replay/...
  - internal/tui/app_trace_packet.go, internal/tui/app_trace_packet_analyzer.go, internal/tui/app_trace_history.go, internal/tui/trace_history_compare.go -> trace/...
  - panel_*.go -> panels/...
  - internal/tui/help.go, internal/tui/interface_stats_explain.go, internal/tui/socket_queue_explain.go -> overlays/...
  - internal/tui/conntrack_percent_format.go, internal/tui/traffic_status_runtime.go, internal/tui/update_check.go -> shared/... hoặc live/... tùy coupling
  - internal/tui/layout.go, internal/tui/history_layout.go -> layout/...

  Refactor Steps

  1. Chuẩn hóa package boundary

  - tạo thư mục mới
  - giữ package name là tui trước
  - chỉ move file, chưa đổi logic lớn

  2. Tách model chung

  - gom các type chung như health level, interface snapshot, trace history entry, hotkey label
  - tránh circular dependency trước khi move sâu hơn

  3. Tách trace

  - move toàn bộ trace sang trace/
  - chuẩn hóa naming: mode, preset, scope, history
  - giữ public seam rõ: promptTracePacket, promptTraceHistory, analyzer helpers

  4. Tách replay

  - move replay app + timeline + trace-history modal
  - replay không nên import logic live trừ shared formatter/render helper

  5. Tách panels

  - panel render phải càng thuần càng tốt
  - mục tiêu: input data in, string out
  - loại bỏ việc panel render tự access app state nếu có

  6. Tách overlays

  - help, explain, modal builders gom một chỗ
  - hotkey copy và help text lấy từ shared source thay vì hardcode nhiều nơi

  7. Chia nhỏ app_core.go

  - còn lại coordinator:
      - NewApp
      - Run
      - goroutine startup
      - focus/page wiring
  - key routing sang live/keys.go
  - refresh path sang live/refresh.go
  - statusbar sang live/statusbar.go

  8. Tổ chức lại test

  - test đi theo module
      - replay/*_test.go
      - panels/*_test.go
  - regression tests giữ ở top-level hoặc live/

  Non-Goals

  - không đổi feature behavior
  - không redesign UI trong refactor
  - không optimize performance trước khi boundary rõ
  - không đổi package name lớn ngay lượt đầu

  Risk Chính


  Cách Giảm Risk

  - giữ package tui trong toàn bộ thư mục con ở giai đoạn đầu
  - move file trước, rename sau
  - sau mỗi cụm move:
      - gofmt
      - go test ./internal/tui -count=1
      - go test ./...
  - chỉ sau khi structure ổn mới rename symbol cho đẹp hơn

  Definition Of Done

  - không còn flat folder 50+ file ở internal/tui
  - internal/tui/app_core.go giảm mạnh, chỉ còn coordinator
  - trace/replay/live/panels có ownership rõ
  - test đọc theo feature được, không phải săn file thủ công
  - không đổi behavior observable của live/replay/trace

  Khuyến Nghị Triển Khai

  - nếu làm thật, nên chia thành 4 PR logic:
      1. folder skeleton + move panels/overlays/layout
      2. move trace
      3. move replay
      4. slim app_core.go và cleanup cuối

• Updated Plan
  └ Refactor will start by extracting low-coupling render/layout/overlay code into real subpackages, then rewiring root tui to use them, while keeping behavior stable.
    □ Audit current root-tui dependencies and identify low-coupling files safe to extract first
    □ Extract panels, overlays, and layout into subpackages with stable APIs
    □ Rewire root tui to use extracted packages and keep tests green
    □ Extract replay and trace helpers where coupling allows without breaking behavior
    □ Run full test suite and summarize remaining coupling for next pass


  Cut tiếp theo (sau wave 8)

  1. Tách diagnosis engine khỏi root: move `top_diagnosis.go` sang `internal/tui/shared` hoặc package chuyên biệt, đồng thời kéo `stateWarnings`/`shortStateName` ra khỏi `panels` nếu cần để tránh ownership sai.
  2. Giảm `shared_aliases.go`: cho root import trực tiếp `shared`/`panels` ở các điểm coupling thấp, rồi xóa dần alias type/value không còn cần thiết.
  3. Tách replay UI glue tiếp: `history_trace_history_modal.go`, `history_trace_timeline.go`, và phần pure của `history_timeline_search.go`/`history_keys.go` sang `internal/tui/replay` với interface hẹp cho `HistoryApp`.
  4. Gộp các root glue file quá mỏng như `app_panel_render.go` vào coordinator tương ứng nếu chúng không tạo seam kỹ thuật thật.
  5. Sau khi bridge giảm đủ, mới slim mạnh `history_app.go` và `app_core.go` xuống đúng coordinator level.
