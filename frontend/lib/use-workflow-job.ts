"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { DocumentWorkflowJob } from "@/lib/types";

const terminal = new Set(["SUCCEEDED", "PARTIAL_FAILED", "FAILED", "CANCELED"]);

export function useWorkflowJob(initialJob?: DocumentWorkflowJob,
  onTerminal?: (job: DocumentWorkflowJob) => void) {
  const [job, setJobState] = useState(initialJob);
  const [pollError, setPollError] = useState("");
  const [watchEpoch, setWatchEpoch] = useState(0);
  const generation = useRef(0);
  const timer = useRef<number | undefined>(undefined);
  const abort = useRef<AbortController | undefined>(undefined);
  const failures = useRef(0);
  const terminalCallback = useRef(onTerminal);
  terminalCallback.current = onTerminal;

  const stop = useCallback(() => {
    if (timer.current !== undefined) window.clearTimeout(timer.current);
    timer.current = undefined;
    abort.current?.abort();
    abort.current = undefined;
  }, []);

  const setJob = useCallback((next?: DocumentWorkflowJob) => {
    generation.current += 1;
    failures.current = 0;
    setPollError("");
    stop();
    setJobState(next);
    setWatchEpoch((value) => value + 1);
  }, [stop]);

  useEffect(() => {
    if (initialJob && (!job || initialJob.id > job.id ||
      (initialJob.id === job.id && initialJob.revision > job.revision))) setJob(initialJob);
  }, [initialJob?.id, initialJob?.revision, setJob]);

  useEffect(() => {
    if (!job || terminal.has(job.status)) return;
    const watchedID = job.id;
    const watchedGeneration = generation.current;
    let disposed = false;

    const schedule = (delay: number) => {
      if (disposed) return;
      if (timer.current !== undefined) window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => void poll(), delay);
    };
    const poll = async () => {
      if (disposed || document.visibilityState === "hidden") return;
      abort.current?.abort();
      const controller = new AbortController();
      abort.current = controller;
      try {
        const response = await fetch(`/api/backend/api/document-workflow-jobs/${watchedID}`, {
          cache: "no-store", headers: { Accept: "application/json" }, signal: controller.signal,
        });
        const payload = (await response.json().catch(() => ({}))) as { job?: DocumentWorkflowJob; error?: string };
        if (!response.ok || !payload.job) throw new Error(payload.error ?? `Status request failed (${response.status})`);
        if (disposed || watchedGeneration !== generation.current || payload.job.id !== watchedID) return;
        failures.current = 0;
        setPollError("");
        setJobState(payload.job);
        if (terminal.has(payload.job.status)) {
          terminalCallback.current?.(payload.job);
          return;
        }
        schedule(1200);
      } catch (caught) {
        if (controller.signal.aborted || disposed || watchedGeneration !== generation.current) return;
        failures.current += 1;
        setPollError(caught instanceof Error ? caught.message : "Could not refresh workflow status.");
        schedule(Math.min(1000 * 2 ** failures.current, 10_000));
      }
    };
    const visible = () => {
      if (document.visibilityState === "visible") schedule(0);
    };
    document.addEventListener("visibilitychange", visible);
    schedule(0);
    return () => {
      disposed = true;
      document.removeEventListener("visibilitychange", visible);
      stop();
    };
  }, [job?.id, job?.status, watchEpoch, stop]);

  return { job, setJob, pollError, active: Boolean(job && !terminal.has(job.status)) };
}
