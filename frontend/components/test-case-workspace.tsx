"use client";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import type {
  BusinessTestCase,
  TestCaseFamily,
  SuiteRelease,
} from "@/lib/types";
import { StatusBadge } from "@/components/status-badge";
const defaults = {
  query: "",
  status: "ALL",
  risk: "ALL",
  sort: "KEY",
  archived: "ACTIVE",
  page: 0,
};
export function TestCaseWorkspace({
  setId,
  testCases,
  canReview = false,
}: {
  setId: number;
  testCases: BusinessTestCase[];
  canReview?: boolean;
}) {
  const [families, setFamilies] = useState<TestCaseFamily[]>([]);
  const [releases, setReleases] = useState<SuiteRelease[]>([]);
  const [filters, setFilters] = useState(defaults);
  const [restored, setRestored] = useState(false);
  const [selected, setSelected] = useState<BusinessTestCase[]>([]);
  const [reviewer, setReviewer] = useState("");
  const [comment, setComment] = useState("");
  const [confirmation, setConfirmation] = useState<"APPROVED" | "REJECTED">();
  const [error, setError] = useState("");
  const [results, setResults] = useState<string[]>([]);
  const [pending, setPending] = useState(false);
  const busy = useRef(false);
  const [epoch, setEpoch] = useState(0);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    try {
      const saved = sessionStorage.getItem(`cases:${setId}`);
      if (saved) setFilters({ ...defaults, ...JSON.parse(saved) });
    } catch {}
    setRestored(true);
  }, [setId]);
  useEffect(() => {
    if (restored) {
      try {
        sessionStorage.setItem(`cases:${setId}`, JSON.stringify(filters));
      } catch {}
    }
  }, [filters, restored, setId]);
  useEffect(() => {
    const abort = new AbortController();
    let timer: number;
    const load = async () => {
      try {
        if (busy.current || document.visibilityState !== "visible") return;
        const responses = await Promise.all(
          ["test-case-families", "suite-releases"].map((route) =>
            fetch(`/api/backend/api/document-sets/${setId}/${route}`, {
              signal: abort.signal,
              cache: "no-store",
            }),
          ),
        );
        if (responses.some((r) => !r.ok))
          throw new Error("Không tải được identities/releases. Thử tải lại.");
        const [f, r] = await Promise.all(responses.map((r) => r.json()));
        if (!abort.signal.aborted) {
          setFamilies(f.families);
          setReleases(r.releases);
          setLoaded(true);
        }
      } catch (caught) {
        if (!abort.signal.aborted) setError((caught as Error).message);
      } finally {
        if (!abort.signal.aborted) timer = window.setTimeout(load, 10000);
      }
    };
    void load();
    return () => {
      abort.abort();
      window.clearTimeout(timer);
    };
  }, [setId, epoch]);
  useEffect(() => {
    if (!loaded) return;
    try {
      const scroll = sessionStorage.getItem(`cases-scroll:${setId}`);
      if (scroll) {
        sessionStorage.removeItem(`cases-scroll:${setId}`);
        window.requestAnimationFrame(() =>
          window.requestAnimationFrame(() =>
            window.scrollTo(0, Number(scroll)),
          ),
        );
      }
    } catch {}
  }, [loaded, setId]);
  const filtered = useMemo(
    () =>
      families
        .filter((f) => {
          const t = f.latest_revision;
          return (
            t &&
            (filters.archived === "ALL" ||
              (filters.archived === "ARCHIVED" ? f.archived : !f.archived)) &&
            (!filters.query ||
              `${f.public_key} ${t.title} ${t.expected_result}`
                .toLowerCase()
                .includes(filters.query.toLowerCase())) &&
            (filters.status === "ALL" || t.status === filters.status) &&
            (filters.risk === "ALL" || t.risk === filters.risk)
          );
        })
        .sort((a, b) =>
          filters.sort === "TITLE"
            ? a.latest_revision!.title.localeCompare(b.latest_revision!.title)
            : filters.sort === "RISK"
              ? { HIGH: 0, MEDIUM: 1, LOW: 2 }[a.latest_revision!.risk] -
                { HIGH: 0, MEDIUM: 1, LOW: 2 }[b.latest_revision!.risk]
              : a.public_key.localeCompare(b.public_key),
        ),
    [families, filters],
  );
  const page = Math.min(
    filters.page,
    Math.max(0, Math.ceil(filtered.length / 20) - 1),
  );
  const visible = filtered.slice(page * 20, page * 20 + 20);
  const link = (t: BusinessTestCase) => (
    <Link
      prefetch={false}
      onClick={() => {
        try {
          sessionStorage.setItem(
            `cases-scroll:${setId}`,
            String(window.scrollY),
          );
        } catch {}
      }}
      href={`/documents/${setId}/test-cases/${t.id}`}
    >
      {t.test_case_key} v{t.version_number} · #{t.id}
    </Link>
  );
  const review = async () => {
    if (!confirmation || busy.current || !reviewer.trim()) return;
    busy.current = true;
    setPending(true);
    setError("");
    const done: number[] = [];
    const messages: string[] = [];
    try {
      for (const item of selected) {
        try {
          const response = await fetch(
            `/api/backend/api/test-cases/${item.id}/review`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                decision: confirmation,
                reviewer_name: reviewer,
                comment,
                expected_content_hash: item.content_hash,
              }),
            },
          );
          const value = await response.json();
          if (!response.ok) throw new Error(value.error);
          done.push(item.id);
          messages.push(
            `#${item.id} v${item.version_number}: đã lưu ${confirmation}`,
          );
        } catch (caught) {
          messages.push(`#${item.id}: ${(caught as Error).message}`);
        }
      }
      setResults(messages);
      setSelected((old) => old.filter((t) => !done.includes(t.id)));
      setConfirmation(undefined);
      setEpoch((v) => v + 1);
      window.dispatchEvent(new Event("document-workspace-updated"));
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  return (
    <section className="panel revision-workspace">
      <div className="panel-header">
        <div>
          <h2>Testcase theo identity</h2>
          <p>
            Latest, approved và revision trong release là các mốc độc lập. Mỗi
            link mở đúng revision.
          </p>
        </div>
        <span>{loaded ? families.length : testCases.length} identities</span>
      </div>
      <div className="filter-strip">
        <input
          aria-label="Tìm testcase"
          placeholder="ID, tiêu đề, expected"
          value={filters.query}
          onChange={(e) =>
            setFilters({ ...filters, query: e.target.value, page: 0 })
          }
        />
        {(
          [
            ["status", "Trạng thái", ["ALL", "DRAFT", "APPROVED", "REJECTED"]],
            ["risk", "Rủi ro", ["ALL", "HIGH", "MEDIUM", "LOW"]],
            ["sort", "Sắp xếp", ["KEY", "TITLE", "RISK"]],
            ["archived", "Lưu trữ", ["ACTIVE", "ARCHIVED", "ALL"]],
          ] as const
        ).map(([key, label, values]) => (
          <label key={key}>
            {label}
            <select
              value={filters[key]}
              onChange={(e) =>
                setFilters({ ...filters, [key]: e.target.value, page: 0 })
              }
            >
              {values.map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </label>
        ))}
      </div>
      {error ? (
        <p role="alert">
          {error}
          <button
            onClick={() => {
              setError("");
              setEpoch((v) => v + 1);
            }}
          >
            Tải lại
          </button>
        </p>
      ) : null}
      <p className="panel-body">
        {filtered.length} kết quả · trang {page + 1} · chọn {selected.length}
        /100 exact revisions (
        {
          selected.filter(
            (s) => !visible.some((f) => f.head_revision_id === s.id),
          ).length
        }{" "}
        ngoài trang này). Không mặc định chọn xuyên trang.
      </p>
      <div className="table-wrap" tabIndex={0} aria-label="Danh sách testcase">
        <table className="data-table">
          <thead>
            <tr>
              <th>Chọn</th>
              <th>Identity / latest</th>
              <th>Approved</th>
              <th>Trong release (pinned)</th>
              <th>Readiness / nguồn</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((f) => {
              const t = f.latest_revision!;
              const pinned = releases.flatMap((r) =>
                r.items
                  .filter((i) => i.family_id === f.id)
                  .map((i) => ({ r, i })),
              );
              return (
                <tr key={f.id}>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`Chọn ${f.public_key} v${t.version_number}`}
                      checked={selected.some((s) => s.id === t.id)}
                      disabled={!canReview || pending || f.archived}
                      onChange={() =>
                        setSelected((old) =>
                          old.some((s) => s.id === t.id)
                            ? old.filter((s) => s.id !== t.id)
                            : old.length < 100
                              ? [...old, t]
                              : old,
                        )
                      }
                    />
                  </td>
                  <td>
                    {link(t)}
                    <p>{t.title}</p>
                    <StatusBadge status={t.status} />
                    {f.archived ? (
                      <p>Đã lưu trữ · history vẫn đọc được</p>
                    ) : null}
                  </td>
                  <td>
                    {f.latest_approved_revision
                      ? link(f.latest_approved_revision)
                      : "Chưa duyệt"}
                  </td>
                  <td>
                    {pinned.length
                      ? pinned.map(({ r, i }) => (
                          <p key={r.id}>
                            <Link
                              prefetch={false}
                              href={`/documents/${setId}/test-cases/${i.test_case_id}`}
                            >
                              R{r.release_number} → v{i.revision_number}
                            </Link>
                          </p>
                        ))
                      : "Chưa chốt"}
                  </td>
                  <td>
                    {t.needs_source_review
                      ? "Cần đối chiếu nguồn"
                      : t.status === "APPROVED"
                        ? "Đã duyệt; publish sẽ kiểm tra nguồn lại"
                        : "Chưa sẵn sàng chốt"}
                    <p>
                      {t.latest_execution ? (
                        <>
                          {t.latest_execution.status} · Run #
                          {t.latest_execution.test_run_id}
                        </>
                      ) : (
                        "Chưa chạy revision này"
                      )}
                    </p>
                    <p>Automation: {t.automation_status}</p>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {!visible.length ? (
        <p className="panel-body">
          {loaded ? "Không có identity khớp bộ lọc." : "Đang tải identities…"}
        </p>
      ) : null}
      <div className="panel-body decision-actions">
        <button
          disabled={!page}
          onClick={() => setFilters({ ...filters, page: page - 1 })}
        >
          Trang trước
        </button>
        <button
          disabled={(page + 1) * 20 >= filtered.length}
          onClick={() => setFilters({ ...filters, page: page + 1 })}
        >
          Trang sau
        </button>
      </div>
      {canReview ? (
        <div className="panel-body">
          <h3>Duyệt exact revisions</h3>
          <div className="connect-grid">
            <label>
              Tên người duyệt
              <input
                value={reviewer}
                onChange={(e) => setReviewer(e.target.value)}
              />
            </label>
            <label>
              Ghi chú
              <input
                value={comment}
                onChange={(e) => setComment(e.target.value)}
              />
            </label>
          </div>
          <button
            className="button"
            disabled={pending || !selected.length}
            onClick={() => setConfirmation("APPROVED")}
          >
            Duyệt nhóm đã chọn
          </button>
          <button
            className="button secondary"
            disabled={pending || !selected.length}
            onClick={() => setConfirmation("REJECTED")}
          >
            Từ chối nhóm
          </button>
          {confirmation ? (
            <section role="dialog" aria-label="Xác nhận revision testcase">
              <h3>
                Xác nhận {confirmation} cho {selected.length} revision
              </h3>
              <ul>
                {selected.map((t) => (
                  <li key={t.id}>
                    {t.test_case_key} v{t.version_number} · #{t.id}
                  </li>
                ))}
              </ul>
              <p>
                Mục thành công giữ nguyên; mục lỗi hiện riêng và vẫn được chọn.
              </p>
              <button
                className="button"
                disabled={pending || !reviewer.trim()}
                onClick={() => void review()}
              >
                Xác nhận duyệt revision
              </button>
              <button
                disabled={pending}
                onClick={() => setConfirmation(undefined)}
              >
                Hủy
              </button>
            </section>
          ) : null}
        </div>
      ) : null}
      {results.length ? (
        <div role="status" className="notice">
          {results.map((v, i) => (
            <p key={i}>{v}</p>
          ))}
        </div>
      ) : null}
    </section>
  );
}
