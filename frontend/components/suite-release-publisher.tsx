"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type {
  BusinessTestCase,
  SuiteRelease,
  TestCaseFamily,
} from "@/lib/types";
type Preview = SuiteRelease & { preview_hash: string };
export function SuiteReleasePublisher({
  setId,
  suiteId,
  testCases,
  releases: initial,
}: {
  setId: number;
  suiteId: number;
  testCases: BusinessTestCase[];
  releases: SuiteRelease[];
}) {
  const [families, setFamilies] = useState<TestCaseFamily[]>([]);
  const [releases, setReleases] = useState(initial);
  const [options, setOptions] = useState<Record<number, BusinessTestCase[]>>(
    {},
  );
  const [cursors, setCursors] = useState<Record<number, number | undefined>>(
    {},
  );
  const [selected, setSelected] = useState<Record<number, BusinessTestCase>>(
    {},
  );
  const [publisher, setPublisher] = useState("");
  const [scope, setScope] = useState("");
  const [preview, setPreview] = useState<Preview>();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const busy = useRef(false);
  const command = useRef<{ body: string; key: string } | undefined>(undefined);
  useEffect(() => {
    const controller = new AbortController();
    fetch(`/api/backend/api/document-sets/${setId}/test-case-families`, {
      signal: controller.signal,
      cache: "no-store",
    })
      .then(async (r) => {
        if (!r.ok)
          throw new Error("Chưa tải được danh sách approved revisions.");
        return r.json();
      })
      .then((value) => {
        setFamilies(
          value.families.filter(
            (f: TestCaseFamily) => f.test_suite_id === suiteId && !f.archived,
          ),
        );
      })
      .catch((caught) => {
        if (!controller.signal.aborted) setError(caught.message);
      });
    return () => controller.abort();
  }, [setId, suiteId, testCases]);
  const older = async (f: TestCaseFamily) => {
    setError("");
    try {
      const r = await fetch(
        `/api/backend/api/test-case-families/${f.id}/history?limit=50&before=${cursors[f.id] ?? 0}`,
      );
      const value = await r.json();
      if (!r.ok) throw new Error(value.error);
      setOptions((old) => ({
        ...old,
        [f.id]: [
          ...(old[f.id] ?? []),
          ...value.entries
            .map((e: { revision: BusinessTestCase }) => e.revision)
            .filter((v: BusinessTestCase) => v.status === "APPROVED"),
        ].filter((v, i, a) => a.findIndex((x) => x.id === v.id) === i),
      }));
      setCursors((old) => ({ ...old, [f.id]: value.next_before }));
    } catch (caught) {
      setError((caught as Error).message);
    }
  };
  const request = async (publish: boolean) => {
    if (busy.current) return;
    const revisions = Object.values(selected);
    if (!revisions.length || !publisher.trim()) {
      setError("Chọn approved revision và nhập tên người chốt.");
      return;
    }
    if (publish && !preview) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const body = JSON.stringify({
        test_suite_id: suiteId,
        revision_ids: revisions.map((v) => v.id),
        published_by: publisher,
        scope_decision: scope,
        ...(publish ? { expected_preview_hash: preview!.preview_hash } : {}),
      });
      if (command.current?.body !== body)
        command.current = { body, key: crypto.randomUUID() };
      const r = await fetch(
        `/api/backend/api/document-sets/${setId}/suite-releases${publish ? "" : "/preview"}`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Idempotency-Key": command.current.key,
          },
          body,
        },
      );
      const value = await r.json();
      if (!r.ok) {
        if (r.status === 409) setPreview(undefined);
        throw new Error(value.error ?? "Chưa chốt được release");
      }
      if (publish) {
        setReleases((old) => [value, ...old.filter((v) => v.id !== value.id)]);
        setPreview(undefined);
        setSelected({});
        command.current = undefined;
        window.dispatchEvent(new Event("document-workspace-updated"));
      } else setPreview(value);
    } catch (caught) {
      setError((caught as Error).message);
    } finally {
      busy.current = false;
      setPending(false);
    }
  };
  return (
    <section className="panel revision-workspace">
      <div className="panel-header">
        <div>
          <h2>Chốt release bất biến</h2>
          <p>
            Mỗi identity chọn một approved revision, kể cả khi latest là draft.
            Không tự chọn tất cả.
          </p>
        </div>
      </div>
      <div className="panel-body">
        <div className="connect-grid">
          <label>
            Tên người chốt
            <input
              value={publisher}
              onChange={(e) => {
                setPublisher(e.target.value);
                setPreview(undefined);
              }}
              maxLength={160}
            />
          </label>
          <label>
            Lý do phạm vi một phần
            <input
              value={scope}
              onChange={(e) => setScope(e.target.value)}
              maxLength={4000}
            />
          </label>
        </div>
      </div>
      <div className="table-wrap" tabIndex={0}>
        <table className="data-table">
          <caption>Chọn exact revision để đưa vào bộ</caption>
          <thead>
            <tr>
              <th>Identity</th>
              <th>Approved revision</th>
              <th>Đọc history</th>
            </tr>
          </thead>
          <tbody>
            {families.map((f) => {
              const values = [
                ...(f.latest_approved_revision
                  ? [f.latest_approved_revision]
                  : []),
                ...(options[f.id] ?? []),
              ].filter((v, i, a) => a.findIndex((x) => x.id === v.id) === i);
              return (
                <tr key={f.id}>
                  <th scope="row">{f.public_key}</th>
                  <td>
                    <select
                      aria-label={`Release revision ${f.public_key}`}
                      value={selected[f.id]?.id ?? ""}
                      disabled={pending}
                      onChange={(e) => {
                        const value = values.find(
                          (v) => v.id === Number(e.target.value),
                        );
                        setSelected((old) => {
                          const next = { ...old };
                          if (value) next[f.id] = value;
                          else delete next[f.id];
                          return next;
                        });
                        setPreview(undefined);
                      }}
                    >
                      <option value="">Không đưa vào bộ</option>
                      {values.map((v) => (
                        <option key={v.id} value={v.id}>
                          v{v.version_number} · #{v.id} · {v.title}
                        </option>
                      ))}
                    </select>
                    {selected[f.id] ? (
                      <Link
                        prefetch={false}
                        href={`/documents/${setId}/test-cases/${selected[f.id].id}`}
                      >
                        Xem revision đã chọn
                      </Link>
                    ) : null}
                  </td>
                  <td>
                    <button
                      disabled={
                        pending ||
                        (options[f.id] !== undefined && !cursors[f.id])
                      }
                      onClick={() => void older(f)}
                    >
                      Tải approved versions cũ
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="panel-body">
        <button
          className="button"
          disabled={pending || !Object.keys(selected).length}
          onClick={() => void request(false)}
        >
          Xem trước phạm vi release
        </button>
        {error ? (
          <p role="alert" className="form-error">
            {error}
          </p>
        ) : null}
        {preview ? (
          <section role="region" aria-label="Preview release">
            <h3>
              Phạm vi {preview.scope_status}:{" "}
              {preview.covered_requirement_count}/
              {preview.approved_requirement_count} requirements
            </h3>
            <ul>
              {preview.items.map((i) => (
                <li key={i.test_case_id}>
                  {i.public_key} v{i.revision_number} · #{i.test_case_id}
                </li>
              ))}
            </ul>
            <p>
              Chưa phủ:{" "}
              {preview.uncovered_requirement_ids.join(", ") || "Không có"} ·
              nguồn #{preview.source_snapshot_id}
            </p>
            <p>
              Server kiểm tra lại revision, approval, nguồn và cùng preview
              token khi chốt. Release cũ không thay đổi.
            </p>
            <button
              className="button"
              disabled={
                pending || (preview.scope_status === "PARTIAL" && !scope.trim())
              }
              onClick={() => void request(true)}
            >
              Xác nhận chốt release
            </button>
          </section>
        ) : null}
        <h3>Release đã chốt</h3>
        {releases.map((r) => (
          <details key={r.id}>
            <summary>
              R{r.release_number} · {r.items.length} revisions ·{" "}
              {r.scope_status}
            </summary>
            <p>
              {r.published_by} · {new Date(r.published_at).toLocaleString()}
            </p>
            <ul>
              {r.items.map((i) => (
                <li key={i.test_case_id}>
                  <Link
                    prefetch={false}
                    href={`/documents/${setId}/test-cases/${i.test_case_id}`}
                  >
                    {i.public_key} v{i.revision_number}
                  </Link>
                </li>
              ))}
            </ul>
          </details>
        ))}
      </div>
    </section>
  );
}
