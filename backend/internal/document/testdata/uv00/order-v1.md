# UC-B08 — Đặt hàng

## Điều kiện trước

- Người mua đã đăng nhập.
- Giỏ hàng có ít nhất một sản phẩm còn hàng.
- Người mua đã có địa chỉ nhận hàng hợp lệ.

## Luồng chính

1. Người mua mở trang xác nhận đơn hàng.
2. Người mua chọn địa chỉ nhận hàng.
3. Người mua nhấn **Đặt hàng**.
4. Hệ thống tạo đơn ở trạng thái `PENDING_PAYMENT` và cấp mã đơn.

## Luồng ngoại lệ

- EX-01: Nếu giỏ hàng rỗng, hệ thống không tạo đơn và hiển thị “Giỏ hàng trống”.
- EX-02: Nếu thiếu địa chỉ nhận hàng, hệ thống không tạo đơn và yêu cầu chọn địa chỉ.

