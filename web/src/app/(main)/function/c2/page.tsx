"use client";

import * as React from "react";

import { PlayIcon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { ToneDot } from "@/components/status-badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { Tone } from "@/lib/status";
import type { SliverBeaconTask, SliverJob, SliverSession, SliverStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

function fmtTime(ts: string) {
  if (!ts || ts.startsWith("1970") || ts === "0001-01-01T00:00:00Z") return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return d.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// Sliver BeaconTaskState enum: 0 pending, 1 sent, 2 completed.
function taskStateMeta(state: string | number) {
  const s = String(state).toLowerCase();
  if (s === "0" || s === "pending") return { label: "Pending", cls: "text-amber-500" };
  if (s === "1" || s === "sent") return { label: "Sent", cls: "text-blue-500" };
  if (s === "2" || s === "completed") return { label: "Completed", cls: "text-emerald-500" };
  return { label: s || "—", cls: "text-muted-foreground" };
}

// Split one command line into argv, honoring "double" and 'single' quotes.
// An unterminated quote yields [] (treated as invalid input).
function splitArgs(line: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quote: string | null = null;
  for (const ch of line) {
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
    } else if (ch === '"' || ch === "'") {
      quote = ch;
    } else if (ch === " " || ch === "\t") {
      if (cur) {
        out.push(cur);
        cur = "";
      }
    } else cur += ch;
  }
  if (quote) return [];
  if (cur) out.push(cur);
  return out;
}

// Listener jobs expose host only via their description ("bound on <host>:<port>…").
function jobHost(job: SliverJob) {
  const m = /bound on ([^\s:]+):(\d+)/.exec(job.description);
  return m?.[1] ?? "";
}

const TABS = [
  { key: "sessions", label: "Sessions" },
  { key: "beacons", label: "Beacons" },
  { key: "listeners", label: "Listeners" },
] as const;
type TabKey = (typeof TABS)[number]["key"];

const LISTENER_TYPES = ["mtls", "http", "https", "dns", "wg"] as const;

export default function C2Page() {
  // ── status banner ──
  const [status, setStatus] = React.useState<SliverStatus | null>(null);
  const [statusError, setStatusError] = React.useState("");
  const [statusLoading, setStatusLoading] = React.useState(true);

  // ── config form ──
  const [config, setConfig] = React.useState("");
  const [configError, setConfigError] = React.useState("");
  const [configSaving, setConfigSaving] = React.useState(false);

  // ── tabs & data ──
  const [tab, setTab] = React.useState<TabKey>("sessions");
  const [sessions, setSessions] = React.useState<SliverSession[]>([]);
  const [beacons, setBeacons] = React.useState<(SliverSession & { interval: number; next_checkin: string })[]>([]);
  const [jobs, setJobs] = React.useState<SliverJob[]>([]);

  // ── session run bar ──
  const [runSession, setRunSession] = React.useState<string | null>(null);
  const [cmd, setCmd] = React.useState("");
  const [runOutput, setRunOutput] = React.useState<string | null>(null);
  const [runError, setRunError] = React.useState("");
  const [running, setRunning] = React.useState(false);

  // ── beacon tasks panel ──
  const [openTasksBeacon, setOpenTasksBeacon] = React.useState<string | null>(null);
  const [tasks, setTasks] = React.useState<SliverBeaconTask[]>([]);
  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set());

  // ── listeners ──
  const [showNewJob, setShowNewJob] = React.useState(false);
  const [jobType, setJobType] = React.useState<(typeof LISTENER_TYPES)[number]>("https");
  const [jobHost_, setJobHost_] = React.useState("0.0.0.0");
  const [jobPort, setJobPort] = React.useState("");
  const [jobDomains, setJobDomains] = React.useState("");
  const [jobWebsite, setJobWebsite] = React.useState("");
  const [jobSubmitting, setJobSubmitting] = React.useState(false);
  const [killJob, setKillJob] = React.useState<SliverJob | null>(null);
  const [killing, setKilling] = React.useState(false);

  const loadStatus = React.useCallback(async () => {
    try {
      const r = await api.sliverStatus();
      setStatus(r);
      setStatusError("");
    } catch (e) {
      setStatusError(String((e as Error)?.message ?? e));
    } finally {
      setStatusLoading(false);
    }
  }, []);

  const loadSessions = React.useCallback(async () => {
    try {
      setSessions(await api.sliverSessions());
    } catch {
      // transient RPC errors leave the previous list in place
    }
  }, []);

  const loadBeacons = React.useCallback(async () => {
    try {
      const r = await api.sliverBeacons();
      setBeacons(r);
    } catch {
      // transient RPC errors leave the previous list in place
    }
  }, []);

  const loadJobs = React.useCallback(async () => {
    try {
      setJobs(await api.sliverJobs());
    } catch {
      // transient RPC errors leave the previous list in place
    }
  }, []);

  const loadTasks = React.useCallback(async (beaconId: string) => {
    try {
      setTasks(await api.sliverBeaconTasks(beaconId));
    } catch {
      // transient RPC errors leave the previous list in place
    }
  }, []);

  React.useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  // Poll the active tab every 5s.
  React.useEffect(() => {
    if (!status?.connected) return;
    const load = { sessions: loadSessions, beacons: loadBeacons, listeners: loadJobs }[tab];
    void load();
    const t = setInterval(() => void load(), 5000);
    return () => clearInterval(t);
  }, [status?.connected, tab, loadSessions, loadBeacons, loadJobs]);

  // Poll the open task list every 5s while it is open.
  React.useEffect(() => {
    if (!openTasksBeacon) return;
    void loadTasks(openTasksBeacon);
    const t = setInterval(() => void loadTasks(openTasksBeacon), 5000);
    return () => clearInterval(t);
  }, [openTasksBeacon, loadTasks]);

  const saveConfig = async () => {
    if (!config.trim()) {
      setConfigError("Operator config is required");
      return;
    }
    setConfigSaving(true);
    setConfigError("");
    try {
      const r = await api.sliverSaveConfig(config.trim());
      setStatus(r);
      setConfig("");
      toast.success("Sliver operator config saved");
    } catch (e) {
      setConfigError(String((e as Error)?.message ?? e));
    } finally {
      setConfigSaving(false);
    }
  };

  const removeConfig = async () => {
    try {
      await api.sliverRemoveConfig();
      await loadStatus();
      setSessions([]);
      setBeacons([]);
      setJobs([]);
      toast.success("Sliver config removed");
    } catch (e) {
      toast.error(String((e as Error)?.message ?? e));
    }
  };

  const runCommand = async () => {
    if (!runSession) return;
    const argv = splitArgs(cmd.trim());
    if (argv.length === 0) {
      setRunError("Empty command or unterminated quote");
      return;
    }
    setRunning(true);
    setRunError("");
    try {
      const r = await api.sliverInteract({ session_id: runSession }, argv);
      setRunOutput(`${r.stdout || ""}${r.stderr || ""}` || `(status ${r.status}, no output)`);
    } catch (e) {
      setRunError(String((e as Error)?.message ?? e));
    } finally {
      setRunning(false);
    }
  };

  const submitJob = async () => {
    const port = Number(jobPort);
    if (!Number.isInteger(port) || port <= 0 || port > 65535) {
      toast.error("Port must be an integer between 1 and 65535");
      return;
    }
    setJobSubmitting(true);
    try {
      const spec = {
        type: jobType,
        host: jobHost_.trim() || "0.0.0.0",
        port,
        ...(jobType === "dns" && jobDomains.trim()
          ? {
              domains: jobDomains
                .split(",")
                .map((d) => d.trim())
                .filter(Boolean),
            }
          : {}),
        ...((jobType === "http" || jobType === "https") && jobWebsite.trim() ? { website: jobWebsite.trim() } : {}),
      };
      if (jobType === "dns" && (!spec.domains || spec.domains.length === 0)) {
        toast.error("DNS listener requires at least one domain");
        return;
      }
      await api.sliverStartListener(spec);
      toast.success(`Listener started on port ${port}`);
      setShowNewJob(false);
      setJobPort("");
      setJobDomains("");
      setJobWebsite("");
      await loadJobs();
    } catch (e) {
      toast.error(String((e as Error)?.message ?? e));
    } finally {
      setJobSubmitting(false);
    }
  };

  const confirmKill = async () => {
    if (!killJob) return;
    setKilling(true);
    try {
      const r = await api.sliverKillJob(killJob.id);
      if (!r.success) throw new Error(`job ${r.id} did not stop`);
      toast.success(`Job ${killJob.id} killed`);
      setKillJob(null);
      await loadJobs();
    } catch (e) {
      toast.error(String((e as Error)?.message ?? e));
    } finally {
      setKilling(false);
    }
  };

  const toggleExpanded = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (statusLoading) {
    return (
      <div className="flex h-[calc(100vh-6rem)] flex-col gap-4 p-1">
        <h1 className="font-semibold text-xl tracking-tight">C2</h1>
        <div className="text-muted-foreground text-sm">Loading…</div>
      </div>
    );
  }

  const configured = status?.configured ?? false;
  const connected = status?.connected ?? false;
  const endpoint = status?.host ? `${status.host}:${status.port ?? "?"}` : "";
  let bannerTone: Tone = "amber";
  if (connected) bannerTone = "green";
  else if (status?.error) bannerTone = "red";
  let bannerText: string;
  if (connected) bannerText = `Connected to ${endpoint}`;
  else if (configured) bannerText = `Not connected (${endpoint})`;
  else bannerText = "Not configured";

  return (
    <div className="flex h-[calc(100vh-6rem)] min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <h1 className="font-semibold text-xl tracking-tight">C2</h1>
        {connected && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              if (tab === "sessions") void loadSessions();
              else if (tab === "beacons") void loadBeacons();
              else void loadJobs();
            }}
          >
            <RefreshCwIcon className="size-4" /> Refresh
          </Button>
        )}
      </div>

      {/* ── status banner ── */}
      {configured ? (
        <Card className="flex items-center justify-between gap-4 px-4 py-3">
          <div className="flex items-center gap-3">
            <ToneDot tone={bannerTone} />
            <div>
              <div className="font-medium text-sm">
                {bannerText}
                {status?.operator ? (
                  <span className="ml-2 text-muted-foreground">operator: {status.operator}</span>
                ) : null}
              </div>
              {status?.error ? <div className="text-red-500 text-xs">{status.error}</div> : null}
              {configured && !connected && (
                <div className="text-muted-foreground text-xs">
                  Connection is retried on the next request. Check that the Sliver server is reachable, or remove the
                  config and re-add it.
                </div>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            {!connected && statusError ? <span className="text-red-500 text-xs">{statusError}</span> : null}
            <Button variant="outline" size="sm" onClick={() => void loadStatus()}>
              Retry
            </Button>
            <Button variant="destructive" size="sm" onClick={() => void removeConfig()}>
              <Trash2Icon className="size-3.5" /> Remove
            </Button>
          </div>
        </Card>
      ) : (
        <Card className="flex flex-col gap-3 px-4 py-4">
          <div>
            <div className="font-medium text-sm">Connect to a Sliver server</div>
            <div className="text-muted-foreground text-xs">
              Paste the operator config JSON produced by{" "}
              <code className="font-mono">sliver new-operator --name artex</code> (or any saved client .cfg).
            </div>
          </div>
          <Textarea
            value={config}
            onChange={(e) => setConfig(e.target.value)}
            placeholder='{"operator":"artex","lhost":"c2.example.com",...}'
            className="min-h-40 font-mono text-xs"
          />
          {configError ? <div className="text-red-500 text-xs">{configError}</div> : null}
          <div>
            <Button size="sm" onClick={() => void saveConfig()} disabled={configSaving}>
              {configSaving ? "Saving…" : "Save"}
            </Button>
          </div>
        </Card>
      )}

      {statusError && !configured ? <div className="text-red-500 text-sm">{statusError}</div> : null}

      {/* ── tabs ── */}
      {connected ? (
        <Tabs value={tab} onValueChange={(v) => setTab(v as TabKey)} className="flex min-h-0 flex-1 flex-col gap-4">
          <div className="overflow-x-auto overflow-y-hidden">
            <TabsList variant="default">
              {TABS.map((t) => (
                <TabsTrigger key={t.key} value={t.key}>
                  {t.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>

          {/* ── sessions ── */}
          <TabsContent value="sessions" className="mt-0 flex min-h-0 flex-1 flex-col gap-3">
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Host / user</TableHead>
                    <TableHead>OS / arch</TableHead>
                    <TableHead>PID</TableHead>
                    <TableHead>Transport</TableHead>
                    <TableHead>Last checkin</TableHead>
                    <TableHead className="w-20 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessions.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-muted-foreground text-sm">
                        No interactive sessions.
                      </TableCell>
                    </TableRow>
                  ) : (
                    sessions.map((s) => (
                      <TableRow key={s.id} className={cn(runSession === s.id && "bg-muted/50")}>
                        <TableCell className="font-medium">{s.name || s.id}</TableCell>
                        <TableCell>
                          {s.hostname || "—"} <span className="text-muted-foreground">/ {s.username || "—"}</span>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {s.os || "—"} / {s.arch || "—"}
                        </TableCell>
                        <TableCell className="tabular-nums">{s.pid || "—"}</TableCell>
                        <TableCell className="text-muted-foreground">{s.transport || "—"}</TableCell>
                        <TableCell className="text-muted-foreground">{fmtTime(s.last_checkin)}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setRunSession(runSession === s.id ? null : s.id);
                              setCmd("");
                              setRunOutput(null);
                              setRunError("");
                            }}
                          >
                            <PlayIcon className="size-3.5" /> {runSession === s.id ? "Close" : "Run"}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </Card>

            {runSession ? (
              <Card className="flex flex-col gap-2 px-4 py-3">
                <div className="flex items-center gap-2">
                  <Input
                    value={cmd}
                    onChange={(e) => setCmd(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !running) void runCommand();
                    }}
                    placeholder="command, e.g. whoami — quotes honored"
                    className="font-mono text-sm"
                    autoFocus
                  />
                  <Button size="sm" onClick={() => void runCommand()} disabled={running}>
                    {running ? "Running…" : "Run"}
                  </Button>
                </div>
                {runError ? <div className="text-red-500 text-xs">{runError}</div> : null}
                {runOutput !== null ? (
                  <pre className="max-h-80 overflow-auto rounded bg-muted p-2 text-xs">{runOutput}</pre>
                ) : null}
              </Card>
            ) : null}
          </TabsContent>

          {/* ── beacons ── */}
          <TabsContent value="beacons" className="mt-0 flex min-h-0 flex-1 flex-col gap-3">
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Host / user</TableHead>
                    <TableHead>OS / arch</TableHead>
                    <TableHead>PID</TableHead>
                    <TableHead>Transport</TableHead>
                    <TableHead>Last checkin</TableHead>
                    <TableHead>Interval</TableHead>
                    <TableHead>Next checkin</TableHead>
                    <TableHead className="w-20 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {beacons.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={9} className="text-muted-foreground text-sm">
                        No beacons.
                      </TableCell>
                    </TableRow>
                  ) : (
                    beacons.map((b) => (
                      <TableRow key={b.id} className={cn(openTasksBeacon === b.id && "bg-muted/50")}>
                        <TableCell className="font-medium">{b.name || b.id}</TableCell>
                        <TableCell>
                          {b.hostname || "—"} <span className="text-muted-foreground">/ {b.username || "—"}</span>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {b.os || "—"} / {b.arch || "—"}
                        </TableCell>
                        <TableCell className="tabular-nums">{b.pid || "—"}</TableCell>
                        <TableCell className="text-muted-foreground">{b.transport || "—"}</TableCell>
                        <TableCell className="text-muted-foreground">{fmtTime(b.last_checkin)}</TableCell>
                        <TableCell className="tabular-nums">{b.interval ? `${b.interval}s` : "—"}</TableCell>
                        <TableCell className="text-muted-foreground">{fmtTime(b.next_checkin)}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              if (openTasksBeacon === b.id) {
                                setOpenTasksBeacon(null);
                                setTasks([]);
                                setExpanded(new Set());
                              } else {
                                setOpenTasksBeacon(b.id);
                                setTasks([]);
                                setExpanded(new Set());
                              }
                            }}
                          >
                            Tasks
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </Card>

            {openTasksBeacon ? (
              <Card className="flex min-h-0 flex-col overflow-hidden py-0">
                <div className="border-b px-4 py-2 font-medium text-sm">
                  Tasks for beacon <span className="font-mono">{openTasksBeacon}</span>
                </div>
                <div className="max-h-96 overflow-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-28">State</TableHead>
                        <TableHead className="w-40">Created</TableHead>
                        <TableHead>Description</TableHead>
                        <TableHead>Response</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {tasks.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={4} className="text-muted-foreground text-sm">
                            No tasks yet.
                          </TableCell>
                        </TableRow>
                      ) : (
                        tasks.map((t) => {
                          const meta = taskStateMeta(t.state);
                          const long = t.response.length > 300;
                          const isExpanded = expanded.has(t.id);
                          return (
                            <TableRow key={t.id}>
                              <TableCell>
                                <span className={cn("font-medium text-xs", meta.cls)}>{meta.label}</span>
                              </TableCell>
                              <TableCell className="text-muted-foreground text-xs">{fmtTime(t.created_at)}</TableCell>
                              <TableCell className="font-mono text-xs">{t.description || "—"}</TableCell>
                              <TableCell className="max-w-md">
                                <pre className="whitespace-pre-wrap text-xs">
                                  {long && !isExpanded ? `${t.response.slice(0, 300)}… ` : t.response || "—"}
                                  {long ? (
                                    <button
                                      type="button"
                                      className="ml-1 text-blue-500 underline"
                                      onClick={() => toggleExpanded(t.id)}
                                    >
                                      {isExpanded ? "Collapse" : "Expand"}
                                    </button>
                                  ) : null}
                                </pre>
                              </TableCell>
                            </TableRow>
                          );
                        })
                      )}
                    </TableBody>
                  </Table>
                </div>
              </Card>
            ) : null}
          </TabsContent>

          {/* ── listeners ── */}
          <TabsContent value="listeners" className="mt-0 flex min-h-0 flex-1 flex-col gap-3">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground text-sm">
                <span className="tabular-nums">{jobs.length}</span> listener{jobs.length === 1 ? "" : "s"}
              </span>
              <Button variant="outline" size="sm" onClick={() => setShowNewJob((v) => !v)}>
                <PlusIcon className="size-3.5" /> {showNewJob ? "Cancel" : "New listener"}
              </Button>
            </div>

            {showNewJob ? (
              <Card className="flex flex-wrap items-end gap-3 px-4 py-4">
                <div className="flex flex-col gap-1">
                  <span className="text-muted-foreground text-xs">Type</span>
                  <Select value={jobType} onValueChange={(v) => setJobType(v as (typeof LISTENER_TYPES)[number])}>
                    <SelectTrigger className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {LISTENER_TYPES.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-muted-foreground text-xs">Host</span>
                  <Input
                    value={jobHost_}
                    onChange={(e) => setJobHost_(e.target.value)}
                    className="w-40 font-mono text-sm"
                  />
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-muted-foreground text-xs">Port</span>
                  <Input
                    value={jobPort}
                    onChange={(e) => setJobPort(e.target.value)}
                    className="w-24 font-mono text-sm"
                    placeholder="443"
                    inputMode="numeric"
                  />
                </div>
                {jobType === "dns" ? (
                  <div className="flex flex-col gap-1">
                    <span className="text-muted-foreground text-xs">Domains (comma-sep)</span>
                    <Input
                      value={jobDomains}
                      onChange={(e) => setJobDomains(e.target.value)}
                      className="w-64 font-mono text-sm"
                      placeholder="tunnel.example.com"
                    />
                  </div>
                ) : null}
                {jobType === "http" || jobType === "https" ? (
                  <div className="flex flex-col gap-1">
                    <span className="text-muted-foreground text-xs">Website (optional)</span>
                    <Input
                      value={jobWebsite}
                      onChange={(e) => setJobWebsite(e.target.value)}
                      className="w-48 font-mono text-sm"
                    />
                  </div>
                ) : null}
                <Button size="sm" onClick={() => void submitJob()} disabled={jobSubmitting}>
                  {jobSubmitting ? "Starting…" : "Start"}
                </Button>
              </Card>
            ) : null}

            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-24">Protocol</TableHead>
                    <TableHead>Host</TableHead>
                    <TableHead>Port</TableHead>
                    <TableHead>Domains</TableHead>
                    <TableHead className="w-24 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {jobs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-muted-foreground text-sm">
                        No listeners running.
                      </TableCell>
                    </TableRow>
                  ) : (
                    jobs.map((j) => (
                      <TableRow key={j.id}>
                        <TableCell>
                          <Badge variant="outline" className="font-mono">
                            {j.protocol || j.name}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono">{jobHost(j) || j.name || "—"}</TableCell>
                        <TableCell className="tabular-nums">{j.port}</TableCell>
                        <TableCell className="font-mono text-muted-foreground">
                          {j.domains && j.domains.length > 0 ? j.domains.join(", ") : "—"}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button variant="destructive" size="sm" onClick={() => setKillJob(j)}>
                            Kill
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </Card>
          </TabsContent>
        </Tabs>
      ) : null}

      {/* ── kill-job confirm ── */}
      <AlertDialog open={killJob !== null} onOpenChange={(open) => !open && setKillJob(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Kill listener?</AlertDialogTitle>
            <AlertDialogDescription>
              {killJob
                ? `This stops ${killJob.protocol || killJob.name} on port ${killJob.port}. Implants using it will lose their channel until another listener is started.`
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void confirmKill()} disabled={killing}>
              {killing ? "Killing…" : "Kill"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
