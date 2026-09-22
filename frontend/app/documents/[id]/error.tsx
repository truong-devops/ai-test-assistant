"use client";
export default function DocumentWorkspaceError({ reset }: { reset: () => void }) {
  return <div className="panel panel-body"><h2>Chưa tải được bộ tài liệu</h2><p role="alert">Kiểm tra kết nối và thử tải lại. Các tác vụ đã gửi vẫn được lưu trên server.</p><button className="button" onClick={reset}>Thử tải lại</button></div>;
}
