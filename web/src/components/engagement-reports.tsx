"use client";

import { useEffect, useState } from "react";

import { Markdown } from "@/components/markdown";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { http } from "@/lib/api";
import {
  collaborationTime,
  downloadCollaboration,
  type EngagementTask,
  engagementPath,
  mutation,
  type SavedReport,
} from "@/lib/collaboration";
import { visiblePoll } from "@/lib/visible-poller";

export function EngagementReports({
  id,
  canGenerate,
  canApprove,
}: {
  id: number;
  canGenerate: boolean;
  canApprove: boolean;
}) {
  const base = engagementPath(id);
  const [reports, setReports] = useState<SavedReport[]>([]);
  const [tasks, setTasks] = useState<EngagementTask[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [title, setTitle] = useState("Engagement report");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [taskPage, setTaskPage] = useState(1);
  const [taskTotal, setTaskTotal] = useState(0);
  const [detail, setDetail] = useState<SavedReport | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(
    () =>
      visiblePoll(async (signal) => {
        try {
          const result = await http<{ items: SavedReport[]; total: number }>(
            `${base}/reports?q=${encodeURIComponent(search)}&status=${status}&page=${page}&limit=20`,
            { signal },
          );
          if (signal.aborted) return;
          setReports(result.items);
          setTotal(result.total);
          setError("");
        } catch (e) {
          if (!signal.aborted) setError((e as Error).message);
        } finally {
          if (!signal.aborted) setLoading(false);
        }
      }, 10000),
    [base, search, status, page, revision],
  );
  useEffect(() => {
    const controller = new AbortController();
    void http<{ items: EngagementTask[]; total: number }>(`${base}/tasks?page=${taskPage}&limit=25`, {
      signal: controller.signal,
    })
      .then((r) => {
        setTasks(r.items);
        setTaskTotal(r.total);
      })
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => controller.abort();
  }, [base, taskPage]);
  async function act(action: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    try {
      await action();
      setRevision((r) => r + 1);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function open(reportId: number) {
    const report = await http<SavedReport>(`${base}/reports/${reportId}`);
    setDetail(report);
  }
  async function generate(previous?: SavedReport) {
    await mutation(`${base}/reports`, "POST", {
      title,
      task_ids: previous?.task_ids ?? selected,
      previous_id: previous?.id ?? 0,
    });
  }
  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
      <div className="space-y-4">
        {canGenerate && (
          <Card>
            <CardHeader>
              <CardTitle>Generate saved AI report</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <Input
                aria-label="Report title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                maxLength={200}
              />
              <fieldset className="space-y-2">
                <legend className="mb-2 font-medium text-sm">Source tasks ({selected.length} selected)</legend>
                {tasks.map((t) => (
                  <label key={t.id} className="flex items-start gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={selected.includes(t.id)}
                      onChange={(e) =>
                        setSelected(e.target.checked ? [...selected, t.id] : selected.filter((x) => x !== t.id))
                      }
                    />
                    <span>
                      {t.name || t.description} <span className="text-muted-foreground">#{t.id}</span>
                    </span>
                  </label>
                ))}
              </fieldset>
              <div className="flex gap-2">
                <Button variant="outline" disabled={taskPage === 1} onClick={() => setTaskPage(taskPage - 1)}>
                  Previous tasks
                </Button>
                <Button
                  variant="outline"
                  disabled={taskPage * 25 >= taskTotal}
                  onClick={() => setTaskPage(taskPage + 1)}
                >
                  Next tasks
                </Button>
              </div>
              <Button disabled={busy || !selected.length || !title.trim()} onClick={() => void act(() => generate())}>
                Generate draft
              </Button>
              <p className="text-muted-foreground text-xs">
                Uses engagement default model and authorized stored records. Generated drafts require manager approval.
              </p>
            </CardContent>
          </Card>
        )}
        <Input
          aria-label="Search reports"
          placeholder="Search reports"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(1);
          }}
        />
        <select
          aria-label="Report status"
          value={status}
          onChange={(event) => {
            setStatus(event.target.value);
            setPage(1);
          }}
          className="rounded-md border bg-background p-2 text-sm"
        >
          <option value="">All statuses</option>
          {["queued", "running", "failed", "completed", "canceled"].map((value) => (
            <option key={value}>{value}</option>
          ))}
        </select>
        {loading && <p role="status">Loading reports…</p>}
        {error && (
          <p role="alert" className="text-destructive">
            {error} Existing data may be stale.
          </p>
        )}
        {!loading && !reports.length && <p>No saved reports.</p>}
        {reports.map((report) => (
          <Card key={report.id}>
            <CardContent className="space-y-2 pt-5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Button variant="link" className="h-auto p-0 text-left" onClick={() => void act(() => open(report.id))}>
                  {report.title} · v{report.version}
                </Button>
                <Badge variant="outline">{report.approved_at ? "Approved" : report.status}</Badge>
              </div>
              <p className="text-muted-foreground text-xs">
                {report.author} · {collaborationTime(report.created_at)} · {report.model}
              </p>
              <p className="text-xs">Tasks: {report.task_ids.join(", ")}</p>
              {report.error && <p className="text-destructive text-sm">{report.error}</p>}
              <div className="flex flex-wrap gap-2">
                {canGenerate && (
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => void act(() => generate(report))}>
                    New version / retry
                  </Button>
                )}
                {canGenerate && ["queued", "running"].includes(report.status) && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => void act(() => mutation(`${base}/reports/${report.id}/cancel`, "POST"))}
                  >
                    Cancel
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>
        ))}
        <div className="flex items-center gap-3">
          <Button variant="outline" disabled={page === 1} onClick={() => setPage(page - 1)}>
            Previous
          </Button>
          <span>Page {page}</span>
          <Button variant="outline" disabled={page * 20 >= total} onClick={() => setPage(page + 1)}>
            Next
          </Button>
        </div>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Report preview</CardTitle>
        </CardHeader>
        <CardContent>
          {!detail ? (
            <p>Select a version to preview its immutable saved output.</p>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap gap-2">
                <Badge>{detail.approved_at ? `Approved v${detail.version}` : "Draft"}</Badge>
                {detail.stale && <Badge variant="destructive">Source records changed</Badge>}
              </div>
              <p className="text-sm">
                {detail.title} · v{detail.version}
                {detail.approved_at && ` · Reviewer ${detail.reviewer_id} · ${collaborationTime(detail.approved_at)}`}
              </p>
              {detail.status === "completed" && (
                <div className="flex flex-wrap gap-2">
                  {canApprove && !detail.approved_at && (
                    <Button
                      disabled={busy}
                      onClick={() =>
                        void act(async () => {
                          await mutation(`${base}/reports/${detail.id}/approve`, "POST");
                          await open(detail.id);
                        })
                      }
                    >
                      Approve version
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    onClick={() =>
                      void act(() =>
                        downloadCollaboration(`${base}/reports/${detail.id}/export`, `report-${detail.id}.md`),
                      )
                    }
                  >
                    Markdown
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() =>
                      void act(() =>
                        downloadCollaboration(
                          `${base}/reports/${detail.id}/export?format=csv`,
                          `report-${detail.id}.csv`,
                        ),
                      )
                    }
                  >
                    Findings CSV
                  </Button>
                  <Button variant="outline" onClick={() => window.print()}>
                    Print / PDF
                  </Button>
                </div>
              )}
              <section className="report-print space-y-3">
                <p className="font-medium">
                  {detail.approved_at
                    ? `APPROVED version ${detail.version} · Reviewer ${detail.reviewer_id} · ${detail.approved_at}`
                    : "DRAFT"}
                </p>
                {detail.output ? (
                  <Markdown text={detail.output} />
                ) : (
                  <p>{detail.error || `Generation ${detail.status}`}</p>
                )}
              </section>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
