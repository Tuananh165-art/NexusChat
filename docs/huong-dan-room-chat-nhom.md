# Room chat nhóm NexusChat

Tài liệu này mô tả kế hoạch, todo triển khai và cách test chức năng room chat nhóm mới.

## 1. Mục tiêu

Room chat nhóm cho phép người dùng:

- Tạo room chat có tên riêng.
- Sinh mã mời để chia sẻ cho nhiều người.
- Join room bằng mã mời.
- Mở room để chat nhiều người trong cùng một channel.
- Tái sử dụng chat text, file upload, pin, search, gallery, Safety và Workspace hiện có.
- Chạy bằng HTTP và `ws://`, không cần HTTPS hoặc WebRTC.

## 2. Kiến trúc triển khai

Chức năng room nhóm được triển khai additive trong `chat-service` vì message pipeline hiện tại đã broadcast theo `channel_id`. Mỗi room là một channel có metadata riêng.

Luồng chính:

1. User tạo room qua `POST /api/chat/rooms`.
2. Chat service tạo `channel_id`, lưu room metadata vào Cassandra và thêm owner vào bảng `channels`.
3. Service sinh `invite_code` 8 ký tự.
4. User khác join bằng `POST /api/chat/rooms/join`.
5. Khi mở room, frontend gọi `POST /api/chat/rooms/{channelId}/open` để lấy channel access token.
6. Frontend reconnect WebSocket `/api/chat?uid=...&access_token=...`.
7. Message group vẫn đi qua chat WebSocket, Safety moderation và Kafka event hiện có.

## 3. API mới

### List room của user

```http
GET /api/chat/rooms
X-User-Id: <current-user-id>
```

Response:

```json
{
  "rooms": [
    {
      "channel_id": "123",
      "name": "Team NexusChat",
      "owner_id": "629636030470815863",
      "invite_code": "ABCD2345",
      "member_count": 3,
      "role": "owner"
    }
  ]
}
```

### Tạo room

```http
POST /api/chat/rooms
X-User-Id: <current-user-id>
Content-Type: application/json
```

Body:

```json
{
  "name": "Team NexusChat",
  "member_ids": []
}
```

### Join room bằng mã mời

```http
POST /api/chat/rooms/join
X-User-Id: <current-user-id>
Content-Type: application/json
```

Body:

```json
{
  "invite_code": "ABCD2345"
}
```

### Mở room

```http
POST /api/chat/rooms/{channelId}/open
X-User-Id: <current-user-id>
```

Response:

```json
{
  "channel_id": "123",
  "access_token": "<channel-jwt>"
}
```

### Rời room

```http
POST /api/chat/rooms/{channelId}/leave
X-User-Id: <current-user-id>
```

Owner chưa thể rời room cho đến khi có chức năng transfer ownership.

## 4. Database

Các bảng Cassandra mới:

- `chat_rooms_by_id`: metadata chính của room.
- `chat_rooms_by_user`: danh sách room theo user.
- `chat_room_invites`: map mã mời sang room.

Room membership vẫn tái sử dụng bảng `channels (id, user_id)`. Role vẫn tái sử dụng `channel_roles`.

## 5. Frontend

UI mới nằm ở header chat:

- Icon `Rooms`.
- Tạo room bằng tên.
- Join room bằng mã mời.
- Copy mã mời.
- Mở room để chuyển WebSocket sang group channel.

End user không cần nhập `channel_id` hoặc token.

## 6. Cách test

### 6.1. Test tạo room

1. Đăng nhập User A.
2. Vào `/chat`.
3. Bấm icon `Rooms`.
4. Nhập tên room, ví dụ:

```text
Team NexusChat
```

5. Bấm nút tạo.

Kết quả kỳ vọng:

- Room xuất hiện trong danh sách.
- UI hiển thị mã mời.
- `POST /api/chat/rooms` trả `201`.

### 6.2. Test join room

1. Copy mã mời từ User A.
2. Mở User B ở browser khác.
3. Vào `/chat`.
4. Bấm icon `Rooms`.
5. Nhập mã mời.
6. Bấm join.

Kết quả kỳ vọng:

- User B thấy room vừa join.
- `POST /api/chat/rooms/join` trả `200`.
- `member_count` tăng.

### 6.3. Test mở room và chat nhóm

1. User A bấm mở room.
2. User B bấm mở cùng room.
3. User A gửi tin nhắn.
4. User B nhận tin nhắn realtime.
5. Thêm User C join bằng mã mời và mở room.
6. User C gửi tin nhắn.

Kết quả kỳ vọng:

- Cả A, B, C đều nhận tin nhắn trong cùng room.
- WebSocket dùng `/api/chat?uid=...&access_token=...`.
- Message vẫn lưu trong Cassandra bảng `messages` theo `channel_id`.
- Safety vẫn kiểm duyệt trước broadcast.

### 6.4. Test Workspace trong room

1. Mở room group.
2. Bấm `Workspace`.
3. Tạo task hoặc bookmark.
4. Mở cùng room ở user khác.

Kết quả kỳ vọng:

- Workspace item gắn với room channel.
- User khác trong room thấy item khi mở Workspace.

### 6.5. Test lỗi phổ biến

Nếu mở room bị `401` hoặc `403`:

- Kiểm tra `localStorage.rc:userid`.
- Hard reload `Ctrl + F5`.
- Kiểm tra user đã join room chưa.

Nếu join room không được:

- Kiểm tra mã mời nhập đúng 8 ký tự.
- Kiểm tra room chưa bị xóa/archive.
- Kiểm tra log `chat-service`.

## 7. Todo hoàn thiện tiếp theo

Đã hoàn thiện trong bản hiện tại:

- Backend API tạo/list/join/open/leave room.
- Cassandra schema room.
- Frontend panel tạo/join/copy/mở room.
- Chat nhóm tái sử dụng WebSocket hiện có.
- Build frontend pass.
- Go test package chat/common/realtime pass.

Nên bổ sung ở phase sau:

- Transfer ownership để owner có thể rời room.
- Quản lý member bằng tên/search thay vì nhập `member_ids`.
- Kick member, rename room, archive room.
- Badge hiển thị tên room hiện tại ở chat header.
- Room list page riêng nếu số lượng room lớn.
- Test E2E với 3 browser context.
