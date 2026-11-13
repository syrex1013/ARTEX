"use client";

import * as React from "react";

import Link from "next/link";

import {
  ActivityIcon,
  ArrowUpRightIcon,
  BugIcon,
  ClockIcon,
  NetworkIcon,
  ShieldCheckIcon,
  TargetIcon,
  ZapIcon,
} from "lucide-react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";

import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { type ChartConfig, ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { http } from "@/lib/api";
import {
  type DailyUsageRow,
  dailyUsage,
  formatTokens as fmtTokens,
  sumUsage,
  tokenBreakdown,
} from "@/lib/dashboard-usage";
import type {
  Activity,
  ConvTokenSummary,
  Finding,
  InterceptPending,
  LLMProfile,
  Settings,
  Stats,
  Task,
  TrafficExchange,
  UsageStats,
} from "@/lib/types";
import { cn } from "@/lib/utils";
import { visiblePoll } from "@/lib/visible-poller";

// ── chart constants ───────────────────────────────────────────────────────────

const dailyTrendConfig = {
  input: { label: "Uncached input", color: "hsl(217 91% 60%)" },
  output: { label: "Output", color: "hsl(263 70% 60%)" },
  cacheRead: { label: "Cache read", color: "hsl(160 60% 45%)" },
} satisfies ChartConfig;

// ── helpers ──────────────────────────────────────────────────────────────────

function fmtRel(ts?: string | number): string {
  if (!ts) return "—";
  const ms = Date.now() - (typeof ts === "number" ? ts * 1000 : Date.parse(ts));
  if (!Number.isFinite(ms)) return "—";
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

const ASSET_TYPE_LABELS: Record<string, string> = {
  root_domain: "Root domain",
  ip: "IP",
  subdomain: "Subdomain",
  app: "App",
  service: "Service",
  endpoint: "Endpoint",
};

const ASSET_COLORS: Record<string, string> = {
  root_domain: "bg-blue-500",
  ip: "bg-indigo-400",
  subdomain: "bg-violet-500",
  app: "bg-orange-400",
  service: "bg-cyan-500",
  endpoint: "bg-emerald-500",
};

function statusColor(code: number): string {
  if (code < 300) return "text-tone-success";
  if (code < 400) return "text-tone-info";
  if (code < 500) return "text-tone-warn";
  return "text-tone-danger";
}

function statusBg(code: number): string {
  if (code < 300) return "bg-tone-success";
  if (code < 400) return "bg-tone-info";
  if (code < 500) return "bg-tone-warn";
  return "bg-tone-danger";
}

// ── sub-components ────────────────────────────────────────────────────────────

function LiveDot({ className }: { className?: string }) {
  return (
    <span
      className={cn("inline-block size-1.5 shrink-0 rounded-full bg-tone-info motion-safe:animate-pulse", className)}
    />
  );
}

function SectionTitle({
  icon: Icon,
  children,
  sub,
}: {
  icon: React.ElementType;
  children: React.ReactNode;
  sub?: React.ReactNode;
}) {
  return (
    <div className="mb-1 flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2 font-semibold text-base">
        <Icon className="size-3.5 text-muted-foreground" />
        {children}
      </div>
      {sub && <span className="text-muted-foreground text-xs">{sub}</span>}
    </div>
  );
}

// ── page ─────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  // data state
  const [tasks, setTasks] = React.useState<Task[]>([]);
  const [findings, setFindings] = React.useState<Finding[]>([]);
  const [stats, setStats] = React.useState<Stats | null>(null);
  const [settings, setSettings] = React.useState<Settings | null>(null);
  const [pending, setPending] = React.useState<InterceptPending[]>([]);
  const [activity, setActivity] = React.useState<Activity[]>([]);
  const [convTokens, setConvTokens] = React.useState<ConvTokenSummary[]>([]);
  const [usageStats, setUsageStats] = React.useState<UsageStats | null>(null);
  const [assetCounts, setAssetCounts] = React.useState<Record<string, number>>({});
  const [trafficTotal, setTrafficTotal] = React.useState<number | null>(null);
  const [traffic, setTraffic] = React.useState<TrafficExchange[]>([]);
  const [llmProfiles, setLLMProfiles] = React.useState<LLMProfile[]>([]);

  const [panelErrors, setPanelErrors] = React.useState<Record<string, string>>({});
  const [tasksLoaded, setTasksLoaded] = React.useState(false);
  const [taskTotal, setTaskTotal] = React.useState(0);
  const [findingTotal, setFindingTotal] = React.useState(0);
  const [tasksByStatus, setTasksByStatus] = React.useState<Record<string, number>>({});
  const [findingsBySev, setFindingsBySev] = React.useState({ critical: 0, high: 0, medium: 0, low: 0 });
  const [taskProfiles, setTaskProfiles] = React.useState<
    {
      llm_profile_id: number | null;
      input_tokens: number;
      output_tokens: number;
      cache_read_tokens: number;
      cache_write_tokens: number;
      task_count: number;
    }[]
  >([]);

  const [legacyDaily, setLegacyDaily] = React.useState<
    {
      llm_profile_id: number | null;
      date: string;
      input_tokens: number;
      output_tokens: number;
      cache_read_tokens: number;
    }[]
  >([]);

  React.useEffect(() => {
    const panel = async <T,>(key: string, request: Promise<T>, apply: (value: T) => void, signal: AbortSignal) => {
      try {
        const value = await request;
        if (signal.aborted) return;
        apply(value);
        setPanelErrors((old) => {
          const next = { ...old };
          delete next[key];
          return next;
        });
      } catch (error) {
        if (!signal.aborted) setPanelErrors((old) => ({ ...old, [key]: (error as Error).message }));
      }
    };
    const stopFast = visiblePoll(async (signal) => {
      await Promise.allSettled([
        panel(
          "findings",
          http<{ total: number; severity: typeof findingsBySev; recent: Finding[] }>("/dashboard/findings", { signal }),
          (value) => {
            setFindings(value.recent);
            setFindingTotal(value.total);
            setFindingsBySev(value.severity);
          },
          signal,
        ),
        panel("stats", http<Stats>("/stats", { signal }), setStats, signal),
        panel("approvals", http<InterceptPending[]>("/intercept/pending", { signal }), setPending, signal),
        panel(
          "activity",
          http<{ items: Activity[] }>("/exploration/activity?limit=30", { signal }),
          (value) => setActivity(value.items),
          signal,
        ),
      ]);
    }, 10000);
    const stopSlow = visiblePoll(async (signal) => {
      await Promise.allSettled([
        panel(
          "tasks",
          http<{
            tasks: Task[];
            total: number;
            counts: Record<string, number>;
            token_profiles: typeof taskProfiles;
            token_daily: typeof legacyDaily;
          }>("/dashboard/tasks", { signal }),
          (value) => {
            setTasks(value.tasks);
            setTaskTotal(value.total);
            setTasksByStatus(value.counts);
            setTaskProfiles(value.token_profiles);
            setLegacyDaily(value.token_daily ?? []);
            setTasksLoaded(true);
          },
          signal,
        ),
        panel("settings", http<Settings>("/settings", { signal }), setSettings, signal),
        panel(
          "conversation tokens",
          http<ConvTokenSummary[]>("/tokens/conversations", { signal }),
          setConvTokens,
          signal,
        ),
        panel(
          "traffic",
          http<{ exchanges?: TrafficExchange[]; total: number }>("/traffic?page=0&size=50", { signal }),
          (value) => {
            setTraffic(value.exchanges ?? []);
            setTrafficTotal(value.total ?? 0);
          },
          signal,
        ),
        panel("assets", http<Record<string, number>>("/assets/counts", { signal }), setAssetCounts, signal),
        panel(
          "models",
          http<{ profiles: LLMProfile[] }>("/llm/profiles", { signal }),
          (value) => setLLMProfiles(value.profiles),
          signal,
        ),
        panel("usage", http<UsageStats>("/tokens/usage?days=365", { signal }), setUsageStats, signal),
      ]);
    }, 30000);
    return () => {
      stopFast();
      stopSlow();
    };
  }, []);

  // ── derived ───────────────────────────────────────────────────────────────

  const sortedTasks = React.useMemo(
    () =>
      [...tasks]
        .sort((a, b) => (b.last_activity_unix ?? b.created_unix ?? 0) - (a.last_activity_unix ?? a.created_unix ?? 0))
        .slice(0, 5),
    [tasks],
  );

  const recentFindings = React.useMemo(
    () => [...findings].sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts)).slice(0, 8),
    [findings],
  );

  const recentActivity = React.useMemo(
    () =>
      [...activity]
        .filter((a) => a.kind !== "usage")
        .sort((a, b) => b.seq - a.seq)
        .slice(0, 6),
    [activity],
  );

  const totalAssets = React.useMemo(() => Object.values(assetCounts).reduce((a, b) => a + b, 0), [assetCounts]);

  // asset type breakdown
  const assetByType = React.useMemo(() => {
    return Object.entries(assetCounts)
      .filter(([, n]) => n > 0)
      .sort(([, a], [, b]) => b - a)
      .slice(0, 8);
  }, [assetCounts]);

  const assetMax = assetByType[0]?.[1] ?? 1;

  // traffic status code breakdown
  const trafficByCodes = React.useMemo(() => {
    const m: Record<number, number> = {};
    for (const e of traffic) {
      const bucket = Math.floor(e.status / 100) * 100;
      m[bucket] = (m[bucket] ?? 0) + 1;
    }
    return Object.entries(m)
      .sort(([a], [b]) => Number(a) - Number(b))
      .map(([code, n]) => ({ code: Number(code), n }));
  }, [traffic]);

  const trafficMax = Math.max(...trafficByCodes.map((x) => x.n), 1);
  const recentTraffic = [...traffic].sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts)).slice(0, 5);

  // system
  const activeProfile = llmProfiles.find((p) => p.is_default);
  const pendingCount = pending.length;

  const [tokenVersion, setTokenVersion] = React.useState<"old" | "new">("new");
  const [tokenTab, setTokenTab] = React.useState("all");
  const [tokenDays, setTokenDays] = React.useState<7 | 30 | 90 | 180 | 365>(30);
  const profileName = (id: number | null) =>
    llmProfiles.find((profile) => Number(profile.id) === id)?.name ?? (id == null ? "" : `Profile #${id}`);
  const toUsage = (row: {
    input_tokens: number | null;
    output_tokens: number | null;
    cache_read_tokens: number | null;
  }) => ({
    input: row.input_tokens ?? 0,
    output: row.output_tokens ?? 0,
    cacheRead: row.cache_read_tokens ?? 0,
  });
  // Preserve unknown and deleted profile names rather than attributing them to today's default.
  const usageRows =
    tokenVersion === "new"
      ? (usageStats?.by_profile ?? []).map((row) => ({
          profile: `name:${row.profile_name}`,
          ...toUsage(row),
        }))
      : [...taskProfiles, ...convTokens].map((row) => ({
          profile: `name:${profileName(row.llm_profile_id)}`,
          ...toUsage(row),
        }));
  const dailyRows: DailyUsageRow[] =
    tokenVersion === "new"
      ? (usageStats?.daily ?? []).map((row) => ({
          date: row.date,
          profile: `name:${row.profile_name}`,
          ...toUsage(row),
        }))
      : legacyDaily.map((row) => ({
          date: row.date,
          profile: `name:${profileName(row.llm_profile_id)}`,
          ...toUsage(row),
        }));
  const profileOptions = [...new Set([...usageRows, ...dailyRows].map((row) => row.profile))].sort();
  const displayedTokens = sumUsage(usageRows, tokenTab);
  const totalTokens = sumUsage(usageRows);
  const dailyTokenData = dailyUsage(dailyRows, tokenDays, tokenTab);
  const periodTotal = dailyTokenData.reduce((total, day) => total + day.input + day.output + day.cacheRead, 0);
  const breakdown = tokenBreakdown(displayedTokens.input, displayedTokens.output, displayedTokens.cacheRead);
  const tokensLoading = tokenVersion === "new" ? usageStats === null : !tasksLoaded;

  // activity kind label
  function kindLabel(a: Activity): string {
    if (a.kind === "tool_use") return a.tool ?? "tool_use";
    if (a.kind === "tool_result") return "tool_result";
    return a.kind;
  }

  function workerBg(w: string): string {
    if (w === "planner") return "bg-tone-accent/10 text-tone-accent border-tone-accent/20";
    if (w === "mainagent") return "bg-cyan-500/10 text-cyan-700 dark:text-cyan-400 border-cyan-500/20";
    return "bg-tone-info/10 text-tone-info border-tone-info/20";
  }

  // ── render ────────────────────────────────────────────────────────────────

  return (
    <div className="mx-auto flex w-full max-w-[1600px] flex-1 flex-col gap-6 pb-8">
      {/* ── Header ── */}
      <div>
        <div>
          <h1 className="font-semibold text-2xl tracking-tight">Overview</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            System-wide activity, findings and usage. Refreshes automatically.
          </p>
        </div>
      </div>

      {Object.entries(panelErrors).map(([panel, error]) => (
        <p key={panel} role="alert" className="text-destructive text-sm">
          {panel}: {error}. Last successful data retained; panel may be stale.
        </p>
      ))}
      {/* ── Row 1: 5 stat cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
        {/* Active tasks */}
        <Card className="min-w-0 gap-3 py-5 [--card-spacing:--spacing(5)]">
          <CardHeader className="pb-0">
            <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
              <TargetIcon className="size-3" /> Active tasks
            </div>
            <div className="flex items-baseline gap-1.5">
              <span className="font-semibold text-3xl tabular-nums tracking-tight">{tasksByStatus.running ?? 0}</span>
              <span className="text-muted-foreground text-xs">/ {taskTotal}</span>
            </div>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-x-2.5 gap-y-0.5 text-xs">
            {(tasksByStatus.running ?? 0) > 0 && (
              <span className="text-tone-info">Exploring {tasksByStatus.running}</span>
            )}
            {(tasksByStatus.paused ?? 0) > 0 && <span className="text-tone-warn">Paused {tasksByStatus.paused}</span>}
            {(tasksByStatus.done ?? 0) > 0 && <span className="text-tone-success">Completed {tasksByStatus.done}</span>}
            {taskTotal === 0 && <span className="text-muted-foreground">No tasks</span>}
          </CardContent>
        </Card>

        {/* Recorded findings */}
        <Card className="min-w-0 gap-3 py-5 [--card-spacing:--spacing(5)]">
          <CardHeader className="pb-0">
            <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
              <BugIcon className="size-3" /> Recorded findings
            </div>
            <div className="font-semibold text-3xl tabular-nums tracking-tight">{findingTotal}</div>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2.5 text-xs">
            <span className="text-rose-700 dark:text-rose-400">Critical {findingsBySev.critical}</span>
            <span className="text-red-700 dark:text-red-400">High {findingsBySev.high}</span>
            <span className="text-amber-700 dark:text-amber-400">Medium {findingsBySev.medium}</span>
            <span className="text-slate-600 dark:text-slate-400">Low {findingsBySev.low}</span>
          </CardContent>
        </Card>

        {/* Asset nodes */}
        <Card className="min-w-0 gap-3 py-5 [--card-spacing:--spacing(5)]">
          <CardHeader className="pb-0">
            <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
              <NetworkIcon className="size-3" /> Asset nodes
            </div>
            <div className="font-semibold text-3xl tabular-nums tracking-tight">{totalAssets}</div>
          </CardHeader>
          <CardContent className="text-muted-foreground text-xs">Shared across tasks</CardContent>
        </Card>

        {/* Traffic exchanges */}
        <Card className="min-w-0 gap-3 py-5 [--card-spacing:--spacing(5)]">
          <CardHeader className="pb-0">
            <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
              <ActivityIcon className="size-3" /> Traffic exchanges
            </div>
            <div className="font-semibold text-3xl tabular-nums tracking-tight">{trafficTotal ?? "—"}</div>
          </CardHeader>
          <CardContent className="flex items-center gap-1.5 text-muted-foreground text-xs">
            {settings?.traffic_capture ? (
              <>
                <LiveDot />
                <span>Recording</span>
              </>
            ) : (
              <span>Capture is off</span>
            )}
          </CardContent>
        </Card>

        {/* LLM usage */}
        <Card className="min-w-0 gap-3 py-5 [--card-spacing:--spacing(5)]">
          <CardHeader className="pb-0">
            <div className="flex items-center gap-1.5 text-muted-foreground text-xs">
              <ZapIcon className="size-3" /> Token usage
            </div>
            <div className="font-semibold text-3xl tabular-nums tracking-tight">
              {tokensLoading ? "—" : fmtTokens(totalTokens.input + totalTokens.output)}
            </div>
          </CardHeader>
          <CardContent className="text-muted-foreground text-xs">
            Lifetime · {tokenVersion === "new" ? "Metered calls" : "Completed runs"}
          </CardContent>
        </Card>
      </div>

      {/* ── Row 2: LLM token usage ── */}
      <Card className="min-w-0 gap-5 p-5 sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="space-y-1">
            <h2 className="flex items-center gap-2 font-semibold text-base">
              <ZapIcon className="size-4 text-muted-foreground" /> LLM token usage
            </h2>
            <p className="max-w-prose text-muted-foreground text-sm">
              {tokenVersion === "new"
                ? "Per-call metering, including interrupted runs. Covers calls recorded since metering was enabled."
                : "Historical completed runs only. Active tasks and conversations; interrupted runs excluded. Dates reflect run completion."}
            </p>
          </div>
          <div className="flex w-full flex-wrap items-end gap-3 sm:w-auto">
            <div className="space-y-1.5">
              <span className="font-medium text-muted-foreground text-xs">Usage source</span>
              <Select
                value={tokenVersion}
                onValueChange={(value) => {
                  setTokenVersion(value as "old" | "new");
                  setTokenTab("all");
                }}
              >
                <SelectTrigger aria-label="Usage source" className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="new">Metered calls</SelectItem>
                  <SelectItem value="old">Completed runs</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="min-w-0 flex-1 space-y-1.5 sm:flex-none">
              <span className="font-medium text-muted-foreground text-xs">Profile</span>
              <Select value={tokenTab} onValueChange={setTokenTab}>
                <SelectTrigger aria-label="Usage profile" className="w-full sm:w-56">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All profiles</SelectItem>
                  {profileOptions.map((profile) => (
                    <SelectItem key={profile} value={profile}>
                      {profile.slice(5) || "Unassigned"}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>

        {/* Body: metrics left + bar chart right */}
        <div className="grid min-w-0 grid-cols-1 gap-6 border-t pt-6 lg:grid-cols-[240px_minmax(0,1fr)] lg:gap-8">
          {/* Left: key metrics */}
          <div className="flex flex-col gap-4">
            {/* Total */}
            <div>
              <div className="text-muted-foreground text-xs">Lifetime tokens</div>
              <div className="mt-0.5 font-bold text-3xl tabular-nums tracking-tight">
                {tokensLoading ? "—" : fmtTokens(breakdown.total)}
              </div>
              <div className="mt-1 text-muted-foreground text-xs">
                {tokenTab === "all" ? "All profiles" : tokenTab.slice(5) || "Unassigned"} · input + output
              </div>
            </div>

            {/* Per-type bars */}
            <div className="space-y-3">
              {(() => {
                // input already includes cache; split into three non-overlapping segments: uncached input + cache hits + output = total.
                const total = breakdown.total;
                return [
                  {
                    label: "Input (uncached)",
                    value: breakdown.input,
                    barColor: dailyTrendConfig.input.color,
                    text: "text-blue-600 dark:text-blue-400",
                  },
                  {
                    label: "Cache hits",
                    value: breakdown.cacheRead,
                    barColor: dailyTrendConfig.cacheRead.color,
                    text: "text-emerald-700 dark:text-emerald-400",
                  },
                  {
                    label: "Output",
                    value: displayedTokens.output,
                    barColor: dailyTrendConfig.output.color,
                    text: "text-violet-700 dark:text-violet-400",
                  },
                ].map(({ label, value, barColor, text }) => {
                  const pct = total > 0 ? (value / total) * 100 : 0;
                  return (
                    <div key={label}>
                      <div className="mb-1 flex items-center justify-between text-xs">
                        <span className="text-muted-foreground">{label}</span>
                        <span className={cn("font-mono font-semibold tabular-nums", text)}>{fmtTokens(value)}</span>
                      </div>
                      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full" style={{ width: `${pct}%`, background: barColor }} />
                      </div>
                    </div>
                  );
                });
              })()}
            </div>

            {/* Cache hit rate */}
            {(() => {
              // input already includes cache → hit rate = cache hits / total input.
              const hitPct = breakdown.hitRate;
              return (
                <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2 text-xs">
                  <span className="text-muted-foreground">Cache hit rate</span>
                  <span
                    className={cn(
                      "font-semibold tabular-nums",
                      hitPct > 50 ? "text-emerald-700 dark:text-emerald-400" : "text-amber-700 dark:text-amber-400",
                    )}
                  >
                    {hitPct}%
                  </span>
                </div>
              );
            })()}
          </div>

          {/* Right: daily bar chart */}
          <div className="flex min-h-0 min-w-0 flex-col">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap gap-x-4 gap-y-2 text-muted-foreground text-xs">
                {(["input", "output", "cacheRead"] as const).map((k) => (
                  <span key={k} className="flex items-center gap-1">
                    <span
                      className="inline-block size-2 rounded-sm"
                      style={{ background: dailyTrendConfig[k].color }}
                    />
                    {dailyTrendConfig[k].label}
                  </span>
                ))}
              </div>
              <div className="flex gap-0.5 rounded-md border bg-muted/30 p-0.5">
                {(
                  [
                    { days: 7, label: "7d" },
                    { days: 30, label: "30d" },
                    { days: 90, label: "3mo" },
                    { days: 180, label: "6mo" },
                    { days: 365, label: "1y" },
                  ] as const
                ).map(({ days, label }) => (
                  <button
                    type="button"
                    key={days}
                    onClick={() => setTokenDays(days)}
                    aria-pressed={tokenDays === days}
                    className={cn(
                      "min-h-8 rounded px-3 py-1 font-medium text-xs transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                      tokenDays === days
                        ? "bg-background text-foreground shadow-sm"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            <p className="mb-3 text-muted-foreground text-sm">
              <span className="font-medium text-foreground tabular-nums">{fmtTokens(periodTotal)}</span> tokens · last{" "}
              {tokenDays} UTC days
            </p>
            {periodTotal === 0 ? (
              <div
                className="flex flex-1 items-center justify-center rounded-lg border bg-muted/10 text-muted-foreground text-xs"
                style={{ minHeight: 180 }}
              >
                {tokensLoading ? "Loading usage…" : "No recorded usage in this period."}
              </div>
            ) : (
              <ChartContainer config={dailyTrendConfig} className="aspect-auto h-[260px] w-full sm:h-[300px]">
                <BarChart data={dailyTokenData} margin={{ top: 4, right: 4, bottom: 0, left: 0 }} maxBarSize={40}>
                  <CartesianGrid vertical={false} />
                  <XAxis
                    dataKey="date"
                    tickFormatter={(date) =>
                      new Date(`${date}T00:00:00Z`).toLocaleDateString(undefined, {
                        month: "short",
                        day: "numeric",
                        timeZone: "UTC",
                      })
                    }
                    minTickGap={32}
                    tickLine={false}
                    axisLine={false}
                    tick={{ fontSize: 12 }}
                    tickMargin={6}
                    interval="preserveStartEnd"
                  />
                  <YAxis
                    tickFormatter={(v) => fmtTokens(Number(v))}
                    tickLine={false}
                    axisLine={false}
                    tick={{ fontSize: 12 }}
                    width={56}
                  />
                  <ChartTooltip
                    cursor={false}
                    content={
                      <ChartTooltipContent
                        indicator="line"
                        formatter={(value, name) => (
                          <div className="flex w-full items-center justify-between gap-4">
                            <span>
                              {(dailyTrendConfig as Record<string, { label: string }>)[String(name)]?.label ??
                                String(name)}
                            </span>
                            <span className="font-mono font-semibold tabular-nums">{fmtTokens(Number(value))}</span>
                          </div>
                        )}
                      />
                    }
                  />
                  <Bar
                    isAnimationActive={false}
                    dataKey="input"
                    stackId="1"
                    fill="var(--color-input)"
                    radius={[0, 0, 0, 0]}
                  />
                  <Bar
                    isAnimationActive={false}
                    dataKey="output"
                    stackId="1"
                    fill="var(--color-output)"
                    radius={[0, 0, 0, 0]}
                  />
                  <Bar
                    isAnimationActive={false}
                    dataKey="cacheRead"
                    stackId="1"
                    fill="var(--color-cacheRead)"
                    radius={[4, 4, 0, 0]}
                  />
                </BarChart>
              </ChartContainer>
            )}
          </div>
        </div>
      </Card>

      {/* ── Row 3: Activity feed | Findings ── */}
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
        {/* Activity feed */}
        <Card className="min-w-0 gap-5 p-5 sm:p-6">
          <div className="mb-1 flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2 font-semibold text-base">
              <ActivityIcon className="size-3.5 text-muted-foreground" />
              Activity feed
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground text-xs">
                Latest {recentActivity.length} of {activity.filter((entry) => entry.kind !== "usage").length} events
              </span>
              <Link
                href="/function/tasks"
                className="flex items-center gap-0.5 text-muted-foreground text-xs hover:text-foreground"
              >
                View tasks <ArrowUpRightIcon className="size-3" />
              </Link>
            </div>
          </div>

          <div className="mb-2.5 flex flex-wrap gap-1.5">
            {(["tool_use", "tool_result", "text", "thinking", "result"] as const).map((kind) => {
              const count = activity.filter((a) => a.kind === kind).length;
              if (count === 0) return null;
              return (
                <span key={kind} className="rounded border bg-muted/20 px-1.5 py-0.5 text-muted-foreground text-xs">
                  {kind} <strong className="text-foreground/70">{count}</strong>
                </span>
              );
            })}
          </div>

          <div className="divide-y">
            {recentActivity.length === 0 ? (
              <div className="py-4 text-center text-muted-foreground text-xs">No activity records</div>
            ) : (
              recentActivity.map((a) => (
                <div key={a.seq} className="flex gap-3 py-4">
                  <span
                    className={cn(
                      "shrink-0 self-start rounded border px-1.5 py-0.5 font-mono text-xs",
                      workerBg(a.worker),
                    )}
                  >
                    {a.worker}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="font-medium text-sm">{kindLabel(a)}</div>
                    <div className="mt-1 line-clamp-2 break-words text-muted-foreground text-sm leading-relaxed">
                      {a.summary}
                    </div>
                  </div>
                  <div className="shrink-0 text-right">
                    <Badge variant="outline" className="px-1.5 py-0 text-muted-foreground text-xs">
                      {a.kind}
                    </Badge>
                    <div className="mt-0.5 text-muted-foreground text-xs tabular-nums">{fmtRel(a.ts)}</div>
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>

        {/* Findings */}
        <Card className="min-w-0 gap-5 p-5 sm:p-6">
          <div className="mb-1 flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2 font-semibold text-base">
              <BugIcon className="size-3.5 text-muted-foreground" />
              Findings
            </div>
            <Link
              href="/function/findings"
              className="flex items-center gap-0.5 text-muted-foreground text-xs hover:text-foreground"
            >
              All <ArrowUpRightIcon className="size-3" />
            </Link>
          </div>

          <div className="divide-y">
            {recentFindings.length === 0 ? (
              <div className="py-4 text-center text-muted-foreground text-xs">No findings</div>
            ) : (
              recentFindings.map((f) => (
                <div key={f.id} className="flex flex-wrap items-start gap-3 py-4 sm:flex-nowrap">
                  <StatusBadge domain="severity" value={f.severity} className="mt-0.5 shrink-0 px-1.5 py-0 text-xs" />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="line-clamp-2 font-medium text-sm leading-relaxed">{f.vulnclass}</span>
                      {f.task_description && (
                        <span className="truncate text-muted-foreground text-xs">{f.task_description}</span>
                      )}
                    </div>
                    <div className="mt-0.5 line-clamp-2 text-muted-foreground text-sm leading-relaxed">{f.summary}</div>
                  </div>
                  <span className="shrink-0 text-muted-foreground text-xs tabular-nums">{fmtRel(f.ts)}</span>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>

      {/* ── Row 4: Task table ── */}
      <Card className="overflow-hidden p-0">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <div className="flex items-center gap-2 font-semibold text-base">
            <ClockIcon className="size-3.5 text-muted-foreground" />
            Tasks
          </div>
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">{taskTotal} tasks</span>
            <Link
              href="/function/tasks"
              className="flex items-center gap-0.5 text-muted-foreground text-xs hover:text-foreground"
            >
              All <ArrowUpRightIcon className="size-3" />
            </Link>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[560px] border-collapse text-sm">
            <thead>
              <tr className="border-b">
                {["Task", "Status", "Created", "Updated"].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-semibold text-muted-foreground text-xs uppercase tracking-widest first:pl-4"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sortedTasks.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-6 text-center text-muted-foreground text-xs">
                    No tasks
                  </td>
                </tr>
              ) : (
                sortedTasks.map((t) => {
                  return (
                    <tr key={t.id} className="border-b transition-colors last:border-0 hover:bg-muted/30">
                      <td className="max-w-[320px] px-4 py-4">
                        <Link href={`/function/tasks/detail?id=${t.id}`} className="group flex flex-col">
                          <span className="truncate font-medium group-hover:underline">
                            {t.name || t.description || `Task ${t.id}`}
                          </span>
                          <span className="mt-0.5 truncate font-mono text-muted-foreground text-xs">{t.id}</span>
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge domain="task" value={t.status} dot className="px-1.5 py-0 text-xs" />
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-muted-foreground">
                        {new Date(t.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-muted-foreground tabular-nums">
                        {fmtRel(t.last_activity_unix ?? t.created_unix)}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* ── Row 5: Asset distribution | Traffic status codes | Interception & pending approvals ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Asset distribution */}
        <Card className="min-w-0 gap-5 p-5 sm:p-6">
          <SectionTitle icon={NetworkIcon} sub="By type">
            Asset distribution
          </SectionTitle>

          {assetByType.length === 0 ? (
            <div className="py-6 text-center text-muted-foreground text-xs">No asset data</div>
          ) : (
            <div className="flex flex-col gap-2">
              {assetByType.map(([type, count]) => (
                <div key={type} className="flex items-center gap-2 text-xs">
                  <span className="w-24 shrink-0 text-right text-muted-foreground text-xs">
                    {ASSET_TYPE_LABELS[type] ?? type}
                  </span>
                  <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                    <div
                      className={cn("h-full rounded-full", ASSET_COLORS[type] ?? "bg-tone-muted")}
                      style={{ width: `${(count / assetMax) * 100}%` }}
                    />
                  </div>
                  <span className="min-w-8 text-right tabular-nums">{count}</span>
                </div>
              ))}
            </div>
          )}

          <div className="mt-3 border-t pt-3 text-muted-foreground text-xs">{totalAssets} nodes in total</div>
        </Card>

        {/* Traffic status codes */}
        <Card className="min-w-0 gap-5 p-5 sm:p-6">
          <SectionTitle icon={ActivityIcon} sub={`Latest ${traffic.length} requests`}>
            Traffic status codes
          </SectionTitle>

          {/* bar chart */}
          {trafficByCodes.length === 0 ? (
            <div className="py-6 text-center text-muted-foreground text-xs">No traffic data</div>
          ) : (
            <>
              <div className="mb-3 flex items-end gap-2" style={{ height: 52 }}>
                {trafficByCodes.map(({ code, n }) => (
                  <div key={code} className="flex flex-1 flex-col items-center gap-1">
                    <span className="text-muted-foreground text-xs tabular-nums">{n}</span>
                    <div
                      className={cn("min-h-1 w-full rounded-sm", statusBg(code), "opacity-80")}
                      style={{ height: Math.max(4, (n / trafficMax) * 36) }}
                    />
                    <span className={cn("font-mono text-xs", statusColor(code))}>{code}</span>
                  </div>
                ))}
              </div>

              <div className="border-t pt-2.5">
                <div className="mb-1.5 text-muted-foreground text-xs">Recent requests</div>
                <div className="flex flex-col gap-1.5">
                  {recentTraffic.map((e) => (
                    <div key={e.id} className="flex items-center gap-1.5 text-xs">
                      <span
                        className={cn(
                          "shrink-0 rounded border px-1 py-0 font-mono text-xs",
                          e.method === "GET"
                            ? "border-tone-success/30 bg-tone-success/10 text-tone-success"
                            : "border-tone-info/30 bg-tone-info/10 text-tone-info",
                        )}
                      >
                        {e.method}
                      </span>
                      <span className="min-w-0 flex-1 truncate text-muted-foreground">
                        {e.host}
                        {e.url.replace(/^https?:\/\/[^/]+/, "").substring(0, 30)}
                      </span>
                      <span className={cn("shrink-0 font-mono text-xs", statusColor(e.status))}>{e.status}</span>
                    </div>
                  ))}
                </div>
              </div>
            </>
          )}
        </Card>

        {/* System status & pending approvals */}
        <Card className="min-w-0 gap-5 p-5 sm:p-6">
          <SectionTitle icon={ShieldCheckIcon}>System status</SectionTitle>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
              <div className="text-muted-foreground text-xs">LLM config</div>
              <Badge
                variant="outline"
                className={cn(
                  "text-xs",
                  stats?.llm_configured
                    ? "border-tone-success/30 bg-tone-success/10 text-tone-success"
                    : "border-tone-danger/30 bg-tone-danger/10 text-tone-danger",
                )}
              >
                {stats?.llm_configured ? "Configured" : "Not configured"}
              </Badge>
            </div>

            <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
              <div className="text-muted-foreground text-xs">Traffic capture</div>
              <Badge
                variant="outline"
                className={cn(
                  "text-xs",
                  settings?.traffic_capture
                    ? "border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-400"
                    : "text-muted-foreground",
                )}
              >
                {settings?.traffic_capture ? "On" : "Off"}
              </Badge>
            </div>

            {activeProfile && (
              <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
                <div className="text-muted-foreground text-xs">Active model</div>
                <span className="font-mono text-xs">{activeProfile.model}</span>
              </div>
            )}
          </div>

          {/* pending approvals */}
          {pendingCount > 0 && (
            <div className="mt-3">
              <div className="mb-1.5 font-medium text-tone-warn text-xs">Pending approvals ({pendingCount})</div>
              <div className="flex flex-col gap-1.5">
                {pending.slice(0, 3).map((p) => (
                  <Link
                    key={p.id}
                    href="/system/intercept/approvals"
                    className="flex items-center justify-between rounded-lg border border-tone-warn/20 bg-tone-warn/5 px-2.5 py-2 hover:bg-tone-warn/10"
                  >
                    <div className="min-w-0">
                      <div className="font-medium text-xs">{p.tool_name}</div>
                      <div className="text-muted-foreground text-xs">{p.agent_name}</div>
                    </div>
                    <ArrowUpRightIcon className="size-3 shrink-0 text-tone-warn" />
                  </Link>
                ))}
                {pendingCount > 3 && (
                  <Link
                    href="/system/intercept/approvals"
                    className="text-center text-muted-foreground text-xs hover:text-foreground"
                  >
                    {pendingCount - 3} more…
                  </Link>
                )}
              </div>
            </div>
          )}

          {pendingCount === 0 && (
            <div className="mt-3 rounded-lg border bg-muted/10 px-3 py-3 text-center text-muted-foreground text-xs">
              <ShieldCheckIcon className="mx-auto mb-1 size-4 text-emerald-500/50" />
              No pending interception approvals
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
