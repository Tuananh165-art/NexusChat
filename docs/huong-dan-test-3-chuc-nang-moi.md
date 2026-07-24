# Hướng dẫn test 3 chức năng mới: Safety, Discovery, Workspace

Tài liệu này hướng dẫn kiểm thử nhanh ba chức năng mới của NexusChat sau khi thay Presence, Notification và WebRTC Call bằng Safety, Discovery và Workspace.

Các chức năng mới chạy qua HTTP và `ws://`, không cần HTTPS, Web Push, VAPID, WebRTC, TURN hoặc cert-manager.

## 1. Chuẩn bị

Mở hai phiên trình duyệt khác nhau:

- User A: Chrome thường.
- User B: Chrome ẩn danh hoặc Edge.

Cho hai user vào `/chat` và match vào cùng một phòng chat.

Nên test bằng trình duyệt ẩn danh hoặc tắt extension nếu Console có các log như `contentscript.js`, `ObjectMultiplex`, `sidebar.content`, vì các log này thường đến từ extension trình duyệt, không phải NexusChat.

Kiểm tra các service trên K8s:

```powershell
kubectl get deploy -n nexuschat
```

Cần có các Deployment mới:

- `safety`
- `discovery`
- `workspace`

Kiểm tra log khi cần:

```powershell
kubectl logs -n nexuschat deploy/safety --tail=100
kubectl logs -n nexuschat deploy/discovery --tail=100
kubectl logs -n nexuschat deploy/workspace --tail=100
```

## 2. Test Safety

Safety là chức năng cho end user nên không yêu cầu nhập `user_id`. UI sẽ tự lấy người đang ở cùng phòng chat và hiển thị tên/avatar.

### 2.1. Báo cáo người dùng

1. User A và User B cùng vào một phòng chat.
2. Ở User A, bấm icon hình khiên `Trust & Safety`.
3. Kiểm tra popup hiển thị tên/avatar của User B.
4. Bấm icon cờ cạnh User B.
5. Nhập lý do báo cáo, ví dụ `spam`, `harassment`, hoặc `scam`.
6. Xác nhận.

Kết quả kỳ vọng:

- UI hiện thông báo đã gửi báo cáo.
- Network tab không có lỗi `401 Unauthorized`.
- Safety service tạo report.
- Log `deploy/safety` không có panic hoặc lỗi Cassandra/Redis.

### 2.2. Chặn người dùng

1. Ở User A, mở `Trust & Safety`.
2. Bấm icon chặn cạnh User B.
3. Xác nhận chặn.

Kết quả kỳ vọng:

- UI hiện thông báo đã chặn User B.
- Safety lưu block relation.
- Match service về sau không nên ghép lại hai user đã block nhau.

Kiểm tra block list bằng DevTools Console:

```javascript
fetch("/api/safety/blocks", {
  headers: {
    Authorization: `Bearer ${localStorage.getItem("rc:accesstoken")}`,
    "X-User-Id": localStorage.getItem("rc:userid")
  }
}).then(r => r.json()).then(console.log)
```

### 2.3. Kiểm tra spam/risk

Gửi nhanh nhiều tin nhắn lặp lại hoặc link đáng ngờ:

```text
click http://scam.example claim prize
```

Kết quả kỳ vọng tùy rule đang cấu hình:

- Risk thấp: tin nhắn vẫn gửi.
- Risk trung bình: tin nhắn được cho phép nhưng có cảnh báo/audit.
- Risk cao: tin nhắn bị chặn trước khi broadcast.
- Flood nghiêm trọng: user bị mute tạm thời.

## 3. Test Discovery

Discovery chạy phía sau flow matching để xếp hạng candidate theo sở thích. End user không cần nhập ID thủ công. Nếu Discovery lỗi hoặc timeout, Match service vẫn fallback về FIFO để chat không bị hỏng.

### 3.1. Xem Discovery profile

Sau khi User A đã vào chat, mở DevTools Console và chạy:

```javascript
fetch("/api/discovery/profile", {
  headers: {
    Authorization: `Bearer ${localStorage.getItem("rc:accesstoken")}`,
    "X-User-Id": localStorage.getItem("rc:userid")
  }
}).then(r => r.json()).then(console.log)
```

Kết quả kỳ vọng:

- API trả profile của user hiện tại.
- Nếu chưa có profile, service trả profile mặc định.
- Không có lỗi `401 Unauthorized`.

### 3.2. Cập nhật sở thích

Chạy trong DevTools Console:

```javascript
fetch("/api/discovery/profile", {
  method: "PUT",
  headers: {
    "Content-Type": "application/json",
    Authorization: `Bearer ${localStorage.getItem("rc:accesstoken")}`,
    "X-User-Id": localStorage.getItem("rc:userid")
  },
  body: JSON.stringify({
    enabled: true,
    languages: ["vi", "en"],
    interests: ["golang", "devops", "ai"],
    avoid_topics: ["spam", "scam"],
    conversation_goal: "learning"
  })
}).then(r => r.json()).then(console.log)
```

Kết quả kỳ vọng:

- API cập nhật thành công.
- Gọi lại `GET /api/discovery/profile` thấy dữ liệu mới.

### 3.3. Kiểm tra matching

1. Cập nhật profile cho User A.
2. Cập nhật profile cho User B với vài interest giống User A.
3. Cho cả hai user vào hàng chờ matching.
4. Quan sát kết quả match.

Kết quả kỳ vọng:

- Match service gọi Safety để loại user bị block hoặc risk cao.
- Match service gọi Discovery để rank candidate.
- Nếu Discovery lỗi, user vẫn match được bằng FIFO fallback.

Kiểm tra log:

```powershell
kubectl logs -n nexuschat deploy/match --tail=100
kubectl logs -n nexuschat deploy/discovery --tail=100
```

## 4. Test Workspace

Workspace cho phép tạo task, note, bookmark trong phòng chat hiện tại. Chức năng này dùng token channel hiện tại nên user phải đang ở trong `/chat`.

### 4.1. Tạo task

1. Vào phòng chat.
2. Bấm icon checklist `Workspace`.
3. Nhập tiêu đề, ví dụ:

```text
Chuẩn bị demo NexusChat
```

4. Bấm nút tạo task.

Kết quả kỳ vọng:

- Task xuất hiện trong popup Workspace.
- Network tab gọi `POST /api/workspace/items` thành công.
- Không còn lỗi `401 Unauthorized`.

### 4.2. Tạo bookmark

1. Mở popup `Workspace`.
2. Nhập tiêu đề, ví dụ:

```text
Ghi nhớ đoạn trao đổi về deployment
```

3. Bấm nút bookmark.

Kết quả kỳ vọng:

- Bookmark xuất hiện trong danh sách.
- Item được lưu vào Workspace service.

### 4.3. Đổi trạng thái

1. Trong Workspace, bấm trạng thái item, ví dụ `todo`.
2. Kiểm tra trạng thái đổi sang `done`.
3. Bấm lại để chuyển về `todo`.

Kết quả kỳ vọng:

- API `PUT /api/workspace/items/{id}/status` thành công.
- UI cập nhật trạng thái đúng.

### 4.4. Xóa item

1. Trong Workspace, bấm nút `×`.
2. Refresh trang.

Kết quả kỳ vọng:

- Item biến mất khỏi danh sách.
- API `DELETE /api/workspace/items/{id}` thành công.
- Item không xuất hiện lại sau refresh.

### 4.5. Test realtime

1. User A và User B cùng mở một phòng chat.
2. User A mở Workspace và tạo task mới.
3. User B mở Workspace.

Kết quả kỳ vọng:

- User B thấy item mới mà không cần reload, hoặc thấy ngay khi mở popup.
- Network tab có WebSocket tới `/api/workspace/ws`.
- WebSocket dùng `ws://` khi trang chạy HTTP.

## 5. Lỗi thường gặp

### 5.1. `/api/workspace/items` trả `401 Unauthorized`

Nguyên nhân thường gặp:

- User chưa vào phòng chat nên chưa có channel token.
- `localStorage` thiếu `rc:accesstoken`.
- `localStorage` thiếu `rc:userid`.
- Trình duyệt vẫn đang dùng bundle frontend cũ sau deploy.

Cách kiểm tra trong DevTools Console:

```javascript
localStorage.getItem("rc:accesstoken")
localStorage.getItem("rc:userid")
```

Cách xử lý:

1. Bấm `Leave`.
2. Vào lại chat để lấy token mới.
3. Hard reload bằng `Ctrl + F5`.
4. Nếu vẫn lỗi, xóa cache site rồi test lại.

### 5.2. Safety không thấy người khác

Nguyên nhân thường gặp:

- Chưa match với user khác.
- User còn lại đã rời phòng.
- API `/api/chat/users` lỗi hoặc token hết hạn.

Cách xử lý:

1. Mở hai browser/user khác nhau.
2. Cho cả hai match lại vào cùng channel.
3. Mở lại popup `Trust & Safety`.

### 5.3. Console có lỗi từ extension

Các log sau thường không phải lỗi NexusChat:

```text
contentscript.js
ObjectMultiplex
sidebar.content
Unchecked runtime.lastError
```

Cách xác nhận:

1. Mở Chrome Incognito.
2. Tắt extension trong Incognito.
3. Test lại `/chat`.

### 5.4. `Mona-Sans.css` trả `402 Payment Required`

Source NexusChat hiện tại không gọi `ray.st/Mona-Sans.css`. Nếu thấy lỗi này:

- Mở bằng Incognito.
- Tắt extension.
- Xóa cache browser.
- Kiểm tra Network tab xem request được inject từ script nào.

## 6. Checklist nghiệm thu

- Safety hiển thị peer bằng tên/avatar, không bắt nhập `user_id`.
- Safety tạo report thành công.
- Safety block user thành công.
- Tin nhắn spam/risk cao được xử lý theo rule.
- Discovery đọc được profile.
- Discovery cập nhật được profile.
- Match vẫn hoạt động khi Discovery lỗi nhờ FIFO fallback.
- Workspace tạo được task.
- Workspace tạo được bookmark.
- Workspace đổi trạng thái item được.
- Workspace xóa item được.
- Workspace realtime qua `ws://` hoạt động giữa hai browser.
- Không còn lỗi `401` khi user đã vào channel hợp lệ.

## 7. Deploy lại sau khi sửa

Commit và push để workflow build/deploy lại:

```powershell
git add frontend/src/components/RealtimePanel.tsx frontend/src/lib/api.ts pkg/common/http_middleware.go docs/huong-dan-test-3-chuc-nang-moi.md docs/README.md
git commit -m "docs: add new services testing guide"
git push
```

Sau khi workflow chạy xong, kiểm tra rollout:

```powershell
kubectl rollout status deploy/web -n nexuschat
kubectl rollout status deploy/safety -n nexuschat
kubectl rollout status deploy/discovery -n nexuschat
kubectl rollout status deploy/workspace -n nexuschat
```

Mở lại:

```text
http://nexuschat.click/chat
```

Hard reload bằng `Ctrl + F5` để chắc chắn trình duyệt dùng frontend bundle mới.
