"use client";

import { Suspense, useEffect, useState } from "react";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import { ArrowUpRightIcon, FolderKanbanIcon, PlusIcon, SearchIcon } from "lucide-react";
import { toast } from "sonner";

import {
  FINDING_STATUSES,
  SEVERITIES,
  VERIFICATION_META,
} from "@/app/(main)/function/findings/_components/findings-table";
import { EngagementDiscussion } from "@/components/engagement-discussion";
import { type Engagement, useEngagement } from "@/components/engagement-provider";
import { EngagementReports } from "@/components/engagement-reports";
import { EngagementSettings } from "@/components/engagement-settings";
import { FindingEvidenceEditor } from "@/components/finding-evidence-editor";
import { StatusBadge, ToneDot } from "@/components/status-badge";
import { TablePagination } from "@/components/table-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useCurrentUser } from "@/hooks/use-current-user";
import { api, http } from "@/lib/api";
import {
  collaborationTime,
  type EngagementTask,
  engagementPath,
  mutation,
  type SnapshotFinding,
} from "@/lib/collaboration";
import { statusMeta, toneDot } from "@/lib/status";
import type { VerificationStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

type View = "overview" | "tasks" | "findings" | "reports" | "discussion" | "settings";
// Reports and discussion keep their own sidebar routes so the active nav item follows the tab.
const VIEW_ROUTES: Record<View, string> = {
  overview: "/engagements",
  tasks: "/engagements?tab=tasks",
  findings: "/engagements?tab=findings",
  reports: "/reports",
  discussion: "/discussion",
  settings: "/engagements?tab=settings",
};
const VIEWS = Object.keys(VIEW_ROUTES) as View[];
const TASK_STATUSES = ["created", "queued", "running", "paused", "done", "failed", "timeout"];
const ALL = "all";

interface Summary {
  tasks: Record<string, number>;
  severity: Record<string, number>;
  findings: Record<string, number>;
  verification: Record<string, number>;
  reports: Record<string, number>;
  approved_reports: number;
  threads: number;
  members: number;
}
interface AuditEntry {
  id: number;
  actor: string | null;
  action: string;
  subject: string;
  created_at: string;
}

const sum = (counts: Record<string, number> = {}) => Object.values(counts).reduce((a, b) => a + b, 0);

export function EngagementWorkspace({ view }: { view?: View }) {
  return (
    // useSearchParams must sit under a Suspense boundary for static export.
    <Suspense fallback={null}>
      <Workspace view={view} />
    </Suspense>
  );
}

function Workspace({ view }: { view?: View }) {
  const { engagement, loading, error, reload, select } = useEngagement();
  const user = useCurrentUser();
  const params = useSearchParams();
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const tab = params.get("tab") as View | null;
  const active: View = view ?? (tab && VIEWS.includes(tab) ? tab : "overview");
  const isAdmin = user.role === "admin";

  if (loading) return <p role="status">Loading engagements…</p>;
  return (
    <div className="space-y-6">
      {error && (
        <p role="alert" className="text-destructive text-sm">
          Could not load engagements: {error}
        </p>
      )}
      {!engagement ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderKanbanIcon />
            </EmptyMedia>
            <EmptyTitle>No engagements yet</EmptyTitle>
            <EmptyDescription>
              {isAdmin
                ? "Create an engagement to group tasks, findings, reports and team discussion under one scope."
                : "Ask an administrator to add you to an engagement."}
            </EmptyDescription>
          </EmptyHeader>
          {isAdmin && (
            <EmptyContent>
              <Button onClick={() => setCreating(true)}>
                <PlusIcon /> New engagement
              </Button>
            </EmptyContent>
          )}
        </Empty>
      ) : (
        <>
          <EngagementHeader engagement={engagement} onCreate={isAdmin ? () => setCreating(true) : undefined} />
          <Tabs value={active} onValueChange={(value) => router.push(VIEW_ROUTES[value as View])}>
            <TabsList className="max-w-full justify-start overflow-x-auto">
              <TabsTrigger value="overview">Overview</TabsTrigger>
              <TabsTrigger value="tasks">Tasks</TabsTrigger>
              <TabsTrigger value="findings">Findings</TabsTrigger>
              <TabsTrigger value="reports">Reports</TabsTrigger>
              <TabsTrigger value="discussion">Discussion</TabsTrigger>
              <TabsTrigger value="settings">Settings</TabsTrigger>
            </TabsList>
            <EngagementContent key={engagement.id} engagement={engagement} active={active} />
          </Tabs>
        </>
      )}
      <CreateEngagementDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(id) => {
          select(id);
          reload();
        }}
      />
    </div>
  );
}

function EngagementHeader({ engagement, onCreate }: { engagement: Engagement; onCreate?: () => void }) {
  return (
    <header className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate font-semibold text-2xl tracking-tight">{engagement.name}</h1>
            <Badge variant="secondary" className="capitalize">
              {engagement.status}
            </Badge>
            {engagement.legacy && <Badge variant="outline">Default</Badge>}
          </div>
          <p className="max-w-prose text-muted-foreground text-sm">
            {engagement.description || "No description yet."}
            {engagement.legacy && " Visible to administrators only; tasks and chats without an engagement land here."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="capitalize">
            You: {engagement.role}
          </Badge>
          {onCreate && (
            <Button size="sm" variant="outline" onClick={onCreate}>
              <PlusIcon /> New engagement
            </Button>
          )}
        </div>
      </div>
      <TestingWindow starts={engagement.starts_on} ends={engagement.ends_on} />
    </header>
  );
}

const DAY = 86_400_000;
function TestingWindow({ starts, ends }: { starts: string | null; ends: string | null }) {
  if (!starts || !ends) {
    return (
      <p className="text-muted-foreground text-xs">
        No testing window set. Managers can add start and end dates in Settings.
      </p>
    );
  }
  const start = new Date(`${starts}T00:00:00`).getTime();
  const end = new Date(`${ends}T23:59:59`).getTime();
  const now = Date.now();
  const total = Math.max(1, Math.round((end - start) / DAY));
  const elapsed = Math.min(total, Math.max(0, Math.floor((now - start) / DAY) + 1));
  const progress = Math.min(1, Math.max(0, (now - start) / (end - start)));
  const days = (n: number) => `${n} ${n === 1 ? "day" : "days"}`;
  let label = `Day ${elapsed} of ${total}`;
  if (now < start) label = `Starts in ${days(Math.ceil((start - now) / DAY))}`;
  else if (now > end) {
    const ago = Math.floor((now - end) / DAY);
    label = ago ? `Ended ${days(ago)} ago` : "Ended today";
  }
  const fmt = (v: number) => new Date(v).toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return (
    <div className="max-w-2xl space-y-1.5">
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground tabular-nums">{fmt(start)}</span>
        <span className="font-medium">{label}</span>
        <span className="text-muted-foreground tabular-nums">{fmt(end)}</span>
      </div>
      <div
        role="progressbar"
        aria-label="Testing window elapsed"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(progress * 100)}
        className="relative h-1.5 overflow-hidden rounded-full bg-muted"
      >
        <div
          className={cn("h-full rounded-full", now > end ? "bg-muted-foreground/50" : "bg-primary")}
          style={{ width: `${progress * 100}%` }}
        />
      </div>
    </div>
  );
}

function EngagementContent({ engagement, active }: { engagement: Engagement; active: View }) {
  const { id, role } = engagement;
  return (
    <>
      <TabsContent value="overview">{active === "overview" && <Overview engagement={engagement} />}</TabsContent>
      <TabsContent value="tasks">
        {active === "tasks" && <TasksTable id={id} canCreate={role !== "viewer"} />}
      </TabsContent>
      <TabsContent value="findings">
        {active === "findings" && <FindingsTable id={id} editable={role !== "viewer"} />}
      </TabsContent>
      <TabsContent value="reports">
        <EngagementReports id={id} canGenerate={role !== "viewer"} canApprove={role === "manager"} />
      </TabsContent>
      <TabsContent value="discussion">
        <EngagementDiscussion id={id} />
      </TabsContent>
      <TabsContent value="settings">
        <EngagementSettings id={id} editable={role === "manager"} />
      </TabsContent>
    </>
  );
}

function useFetch<T>(path: string | null, revision = 0) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refetches after an explicit save.
  useEffect(() => {
    if (!path) return;
    const controller = new AbortController();
    setLoading(true);
    void http<T>(path, { signal: controller.signal })
      .then((value) => {
        setData(value);
        setError("");
      })
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [path, revision]);
  return { data, error, loading };
}

function Overview({ engagement }: { engagement: Engagement }) {
  const base = engagementPath(engagement.id);
  const { data, error, loading } = useFetch<Summary>(`${base}/summary`);
  const audit = useFetch<AuditEntry[]>(engagement.role === "manager" ? `${base}/audit?limit=8` : null);
  if (loading && !data) return <p role="status">Loading overview…</p>;
  if (error)
    return (
      <p role="alert" className="text-destructive text-sm">
        Could not load the overview: {error}
      </p>
    );
  if (!data) return null;
  const findings = sum(data.severity);
  const verified = data.verification.verified ?? 0;
  const tasks = sum(data.tasks);
  const reports = sum(data.reports);
  const inFlight = (data.reports.queued ?? 0) + (data.reports.running ?? 0);
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Findings by severity</CardTitle>
          <CardDescription>
            {findings
              ? `${findings} findings, ${verified} verified. Select a severity to review it.`
              : "No findings recorded yet. They appear here as tasks report them."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <SeverityBar counts={data.severity} total={findings} />
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SEVERITIES.map((severity) => (
              <Link
                key={severity}
                href={`/engagements?tab=findings&severity=${severity}`}
                className="rounded-md border p-3 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
              >
                <div className="font-semibold text-2xl tabular-nums">{data.severity[severity] ?? 0}</div>
                <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
                  <span className={cn("size-2 rounded-full", severityDot(severity))} />
                  {statusMeta("severity", severity).label}
                </div>
              </Link>
            ))}
          </div>
          {findings > 0 && (
            <div className="space-y-2">
              <h3 className="font-medium text-sm">Disposition</h3>
              <ul className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
                {FINDING_STATUSES.filter((s) => data.findings[s]).map((status) => (
                  <li key={status}>
                    <Link
                      href={`/engagements?tab=findings&status=${status}`}
                      className="flex items-center justify-between border-b border-dashed py-1 hover:text-foreground"
                    >
                      <span className="flex items-center gap-2 text-muted-foreground">
                        <ToneDot tone={statusMeta("finding", status).tone} />
                        {statusMeta("finding", status).label}
                      </span>
                      <span className="tabular-nums">{data.findings[status]}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </CardContent>
      </Card>
      <div className="grid content-start gap-4">
        <Card>
          <CardHeader>
            <CardTitle>
              <Link href="/engagements?tab=tasks" className="hover:underline">
                Tasks
              </Link>
            </CardTitle>
            <CardDescription>{tasks ? `${tasks} in this engagement` : "No tasks assigned yet."}</CardDescription>
          </CardHeader>
          {tasks > 0 && (
            <CardContent>
              <CountList domain="task" counts={data.tasks} order={TASK_STATUSES} />
            </CardContent>
          )}
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>
              <Link href="/reports" className="hover:underline">
                Reports
              </Link>
            </CardTitle>
            <CardDescription>
              {reports
                ? `${data.approved_reports} approved of ${reports}${inFlight ? `, ${inFlight} generating` : ""}`
                : "No reports generated yet."}
            </CardDescription>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>
              <Link href="/discussion" className="hover:underline">
                Team
              </Link>
            </CardTitle>
            <CardDescription>
              {data.members} {data.members === 1 ? "member" : "members"}, {data.threads}{" "}
              {data.threads === 1 ? "discussion thread" : "discussion threads"}
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
      {engagement.role === "manager" && (
        <Card className="lg:col-span-3">
          <CardHeader>
            <CardTitle>Recent activity</CardTitle>
          </CardHeader>
          <CardContent>
            {audit.error && (
              <p role="alert" className="text-destructive text-sm">
                {audit.error}
              </p>
            )}
            {!audit.error && !audit.data?.length && (
              <p className="text-muted-foreground text-sm">{audit.loading ? "Loading…" : "No activity yet."}</p>
            )}
            <ol className="divide-y">
              {audit.data?.map((entry) => (
                <li key={entry.id} className="flex flex-wrap items-baseline justify-between gap-2 py-2 text-sm">
                  <span>
                    <span className="font-medium">{entry.actor ?? "System"}</span> {describeAudit(entry)}
                  </span>
                  <time className="text-muted-foreground text-xs tabular-nums" dateTime={entry.created_at}>
                    {collaborationTime(entry.created_at)}
                  </time>
                </li>
              ))}
            </ol>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function severityDot(severity: string) {
  // Standalone severity dots need the fill colour; toneDot[rose] is white for on-fill contrast.
  return severity === "critical" ? "bg-tone-critical" : toneDot[statusMeta("severity", severity).tone];
}

function SeverityBar({ counts, total }: { counts: Record<string, number>; total: number }) {
  if (!total) return <div className="h-3 rounded-full bg-muted" />;
  return (
    <div className="flex h-3 gap-0.5 overflow-hidden rounded-full" role="img" aria-label="Severity distribution">
      {SEVERITIES.filter((s) => counts[s]).map((severity) => (
        <Link
          key={severity}
          href={`/engagements?tab=findings&severity=${severity}`}
          className={cn("h-full transition-opacity hover:opacity-80", severityDot(severity))}
          style={{ flexGrow: counts[severity] }}
          title={`${counts[severity]} ${statusMeta("severity", severity).label}`}
        />
      ))}
    </div>
  );
}

function CountList({ domain, counts, order }: { domain: "task"; counts: Record<string, number>; order: string[] }) {
  const keys = [...order.filter((k) => counts[k]), ...Object.keys(counts).filter((k) => !order.includes(k))];
  return (
    <ul className="space-y-1 text-sm">
      {keys.map((key) => (
        <li key={key} className="flex items-center justify-between">
          <span className="flex items-center gap-2 text-muted-foreground">
            <ToneDot tone={statusMeta(domain, key).tone} />
            {statusMeta(domain, key).label}
          </span>
          <span className="tabular-nums">{counts[key]}</span>
        </li>
      ))}
    </ul>
  );
}

const AUDIT_TEXT: Record<string, (subject: string) => string> = {
  "engagement.create": () => "created the engagement",
  "engagement.update": () => "updated engagement details",
  "membership.set": (s) => `set membership ${s.replace(":", " to ")}`,
  "membership.remove": (s) => `removed user #${s}`,
  "task.assign": (s) => `moved task #${s} into this engagement`,
  "finding.evidence": (s) => `updated evidence on finding #${s}`,
  "report.generate": (s) => `generated report #${s}`,
  "report.approve": (s) => `approved report #${s}`,
  "report.cancel": (s) => `canceled report #${s}`,
  "thread.create": (s) => `started the thread “${s}”`,
  "chat.cancel": () => "canceled an AI reply",
  "attachment.upload": () => "uploaded an attachment",
};
function describeAudit({ action, subject }: AuditEntry) {
  return AUDIT_TEXT[action]?.(subject) ?? `${action}${subject ? ` ${subject}` : ""}`;
}

function useDebounced(value: string, delay = 300) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

function usePaged<T>(base: string, filters: Record<string, string>, revision = 0) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const query = new URLSearchParams(Object.entries(filters).filter(([, v]) => v && v !== ALL)).toString();
  // biome-ignore lint/correctness/useExhaustiveDependencies: a filter change returns to the first page.
  useEffect(() => setPage(1), [query]);
  const { data, error, loading } = useFetch<{ items: T[]; total: number }>(
    `${base}?page=${page}&limit=${pageSize}${query ? `&${query}` : ""}`,
    revision,
  );
  return { data, error, loading, page, setPage, pageSize, setPageSize };
}

function Toolbar({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2 border-b p-3">{children}</div>;
}

function SearchBox({ value, onChange, label }: { value: string; onChange: (v: string) => void; label: string }) {
  return (
    <div className="relative min-w-48 flex-1 sm:max-w-xs">
      <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        aria-label={label}
        placeholder={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-8 pl-8"
      />
    </div>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger size="sm" className="w-40" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{label}: all</SelectItem>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function TableState({ cols, loading, error, empty }: { cols: number; loading: boolean; error: string; empty: string }) {
  return (
    <TableRow>
      <TableCell
        colSpan={cols}
        className={cn("py-10 text-center text-sm", error ? "text-destructive" : "text-muted-foreground")}
      >
        {error ? (
          <span role="alert">{error}</span>
        ) : (
          <span role={loading ? "status" : undefined}>{loading ? "Loading…" : empty}</span>
        )}
      </TableCell>
    </TableRow>
  );
}

function TasksTable({ id, canCreate }: { id: number; canCreate: boolean }) {
  const user = useCurrentUser();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState(ALL);
  const [revision, setRevision] = useState(0);
  const q = useDebounced(search);
  const list = usePaged<EngagementTask>(`${engagementPath(id)}/tasks`, { q, status }, revision);
  const rows = list.data?.items ?? [];
  const admin = user.role === "admin";
  const cols = admin ? 5 : 4;
  return (
    <Card className="gap-0 py-0">
      <Toolbar>
        <SearchBox label="Search tasks" value={search} onChange={setSearch} />
        <FilterSelect
          label="Status"
          value={status}
          onChange={setStatus}
          options={TASK_STATUSES.map((s) => ({ value: s, label: statusMeta("task", s).label }))}
        />
        <span className="ml-auto text-muted-foreground text-xs tabular-nums">{list.data?.total ?? 0} tasks</span>
        {canCreate && <NewEngagementTaskButton engagementId={id} onCreated={() => setRevision((v) => v + 1)} />}
      </Toolbar>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-16">ID</TableHead>
            <TableHead>Task</TableHead>
            <TableHead className="w-32">Status</TableHead>
            <TableHead className="hidden w-44 md:table-cell">Created</TableHead>
            {admin && <TableHead className="w-12" />}
          </TableRow>
        </TableHeader>
        <TableBody>
          {(list.error || !rows.length) && (
            <TableState
              cols={cols}
              loading={list.loading}
              error={list.error}
              empty={q || status !== ALL ? "No tasks match these filters." : "No tasks in this engagement yet."}
            />
          )}
          {!list.error &&
            rows.map((task) => (
              <TableRow key={task.id}>
                <TableCell className="text-muted-foreground tabular-nums">#{task.id}</TableCell>
                <TableCell className="max-w-0 whitespace-normal">
                  <div className="truncate font-medium">{task.name || task.description || `Task ${task.id}`}</div>
                  {task.goal && <div className="line-clamp-1 text-muted-foreground text-xs">{task.goal}</div>}
                </TableCell>
                <TableCell>
                  <StatusBadge domain="task" value={task.status} dot />
                </TableCell>
                <TableCell className="hidden text-muted-foreground text-xs md:table-cell">
                  {collaborationTime(task.created_at)}
                </TableCell>
                {admin && (
                  <TableCell>
                    <Button asChild variant="ghost" size="icon" className="size-7">
                      <Link
                        href={`/function/tasks/detail?id=${task.id}`}
                        aria-label={`Open task ${task.id} in the console`}
                      >
                        <ArrowUpRightIcon />
                      </Link>
                    </Button>
                  </TableCell>
                )}
              </TableRow>
            ))}
        </TableBody>
      </Table>
      <TablePagination
        page={list.page}
        pageSize={list.pageSize}
        total={list.data?.total ?? 0}
        onPageChange={list.setPage}
        onPageSizeChange={list.setPageSize}
        pageSizeOptions={[25, 50, 100]}
      />
    </Card>
  );
}

function NewEngagementTaskButton({ engagementId, onCreated }: { engagementId: number; onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        <PlusIcon /> New task
      </Button>
      <CreateEngagementTaskDialog
        open={open}
        onOpenChange={setOpen}
        engagementId={engagementId}
        onCreated={onCreated}
      />
    </>
  );
}

// Compact task creation scoped to this engagement. The full console form (templates,
// categories, credentials, advanced options) stays on the Tasks page; tasks created here
// default to the engagement and run concurrently with its other tasks.
function CreateEngagementTaskDialog({
  open,
  onOpenChange,
  engagementId,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  engagementId: number;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [goal, setGoal] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function create() {
    setBusy(true);
    setError("");
    try {
      const task = await api.createTask({
        engagementId,
        name: name.trim(),
        description: description.trim(),
        goal: goal.trim(),
      });
      setName("");
      setDescription("");
      setGoal("");
      onOpenChange(false);
      toast.success(`Task #${task.id} created; it starts alongside the engagement's other tasks.`);
      onCreated();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void create();
          }}
          className="space-y-4"
        >
          <DialogHeader>
            <DialogTitle>New task</DialogTitle>
            <DialogDescription>
              The task joins this engagement and runs concurrently with its other tasks.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="engagement-task-name">Name (optional)</Label>
            <Input
              id="engagement-task-name"
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="External surface recon"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="engagement-task-description">Description</Label>
            <Textarea
              id="engagement-task-description"
              rows={3}
              required
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Target scope, background and constraints"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="engagement-task-goal">Goal</Label>
            <Textarea
              id="engagement-task-goal"
              rows={2}
              required
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="What success looks like, e.g. 'Enumerate all exposed admin panels'"
            />
          </div>
          {error && (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !description.trim() || !goal.trim()}>
              {busy ? "Creating…" : "Create task"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function FindingsTable({ id, editable }: { id: number; editable: boolean }) {
  const params = useSearchParams();
  const [search, setSearch] = useState("");
  const [severity, setSeverity] = useState(params.get("severity") ?? ALL);
  const [status, setStatus] = useState(params.get("status") ?? ALL);
  const [revision, setRevision] = useState(0);
  const q = useDebounced(search);
  const list = usePaged<SnapshotFinding>(`${engagementPath(id)}/findings`, { q, severity, status }, revision);
  const rows = list.data?.items ?? [];
  return (
    <Card className="gap-0 py-0">
      <Toolbar>
        <SearchBox label="Search findings" value={search} onChange={setSearch} />
        <FilterSelect
          label="Severity"
          value={severity}
          onChange={setSeverity}
          options={SEVERITIES.map((s) => ({ value: s, label: statusMeta("severity", s).label }))}
        />
        <FilterSelect
          label="Disposition"
          value={status}
          onChange={setStatus}
          options={FINDING_STATUSES.map((s) => ({ value: s, label: statusMeta("finding", s).label }))}
        />
        <span className="ml-auto text-muted-foreground text-xs tabular-nums">{list.data?.total ?? 0} findings</span>
      </Toolbar>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-24">Severity</TableHead>
            <TableHead>Finding</TableHead>
            <TableHead className="w-32">Disposition</TableHead>
            <TableHead className="hidden w-32 lg:table-cell">Verification</TableHead>
            <TableHead className="hidden w-20 md:table-cell">Task</TableHead>
            <TableHead className="w-40 text-right">Evidence</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(list.error || !rows.length) && (
            <TableState
              cols={6}
              loading={list.loading}
              error={list.error}
              empty={
                q || severity !== ALL || status !== ALL
                  ? "No findings match these filters."
                  : "No findings in this engagement yet."
              }
            />
          )}
          {!list.error &&
            rows.map((finding) => {
              const verification = VERIFICATION_META[finding.verification_status as VerificationStatus];
              return (
                <TableRow key={finding.id}>
                  <TableCell>
                    <StatusBadge domain="severity" value={finding.severity} />
                  </TableCell>
                  <TableCell className="max-w-0 whitespace-normal">
                    <div className="truncate font-medium">{finding.name || `Finding ${finding.id}`}</div>
                    {finding.summary && (
                      <div className="line-clamp-1 text-muted-foreground text-xs">{finding.summary}</div>
                    )}
                    {finding.verification_note && (
                      <div className="line-clamp-1 text-muted-foreground text-xs italic">
                        {finding.verification_note}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>
                    <StatusBadge domain="finding" value={finding.status} />
                  </TableCell>
                  <TableCell className="hidden lg:table-cell">
                    {verification ? (
                      <span className="flex items-center gap-1.5 text-xs">
                        <ToneDot tone={verification.tone} />
                        {verification.label}
                      </span>
                    ) : (
                      <span className="text-muted-foreground text-xs">{finding.verification_status || "—"}</span>
                    )}
                  </TableCell>
                  <TableCell className="hidden text-muted-foreground tabular-nums md:table-cell">
                    #{finding.task_id}
                  </TableCell>
                  <TableCell className="text-right">
                    <FindingEvidenceEditor
                      engagementId={id}
                      findingId={finding.id}
                      editable={editable}
                      onSaved={() => setRevision((value) => value + 1)}
                    />
                  </TableCell>
                </TableRow>
              );
            })}
        </TableBody>
      </Table>
      <TablePagination
        page={list.page}
        pageSize={list.pageSize}
        total={list.data?.total ?? 0}
        onPageChange={list.setPage}
        onPageSizeChange={list.setPageSize}
        pageSizeOptions={[25, 50, 100]}
      />
    </Card>
  );
}

function CreateEngagementDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (id: number) => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function create() {
    setBusy(true);
    setError("");
    try {
      const { id } = await mutation<{ id: number }>("/collab/engagements", "POST", {
        name: name.trim(),
        description: description.trim(),
      });
      setName("");
      setDescription("");
      onOpenChange(false);
      onCreated(id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void create();
          }}
          className="space-y-4"
        >
          <DialogHeader>
            <DialogTitle>New engagement</DialogTitle>
            <DialogDescription>You become its manager. Add members and dates in Settings afterwards.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="engagement-name">Name</Label>
            <Input
              id="engagement-name"
              autoFocus
              required
              maxLength={200}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Acme external pentest, Q4"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="engagement-description">Description</Label>
            <Textarea
              id="engagement-description"
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Scope, objectives and rules of engagement"
            />
          </div>
          {error && (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy ? "Creating…" : "Create engagement"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
