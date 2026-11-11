"use client";

import * as React from "react";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import { ArrowLeftIcon, ClockIcon, PencilIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import type { EngagementSummary } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ENGAGEMENT_STATUSES, EngagementDialog } from "../_components/engagement-form";
import { ActivityTab, describeEvent } from "./_tabs/activity-tab";
import { ScopeTab } from "./_tabs/scope-tab";
import { TasksTab } from "./_tabs/tasks-tab";

const SEVERITIES = ["critical", "high", "medium", "low"] as const;
const SEV_BAR: Record<string, string> = {
  critical: "bg-red-600",
  high: "bg-orange-500",
  medium: "bg-amber-400",
  low: "bg-sky-500",
};

function Kpi({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className="pb-1">
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl">{value}</CardTitle>
      </CardHeader>
      {sub && <CardContent className="text-muted-foreground text-xs">{sub}</CardContent>}
    </Card>
  );
}

function Overview({ s }: { s: EngagementSummary }) {
  const sev = s.findings.by_severity;
  const maxSev = Math.max(1, ...SEVERITIES.map((k) => sev[k] ?? 0));
  const covPct = s.coverage.denominator > 0 ? (s.coverage.tested / s.coverage.denominator) * 100 : 0;
  const retests = s.retests;
  const usedPct = s.budget.used_pct ?? 0;
  const w = s.window;

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi
          label="Tasks"
          value={s.tasks.length}
          sub={Object.entries(s.task_states)
            .map(([k, v]) => `${v} ${k}`)
            .join(" · ")}
        />
        <Kpi label="Findings" value={s.findings.total} sub={`${sev.critical ?? 0} critical · ${sev.high ?? 0} high`} />
        <Kpi
          label="Coverage"
          value={s.coverage.denominator > 0 ? `${covPct.toFixed(0)}%` : "–"}
          sub={
            s.coverage.denominator > 0
              ? `${s.coverage.tested} of ${s.coverage.denominator} assets tested`
              : "no coverage data yet"
          }
        />
        <Kpi
          label="Cost"
          value={`€${(s.usage?.cost_eur ?? 0).toFixed(2)}`}
          sub={
            s.usage && s.usage.unpriced_tokens > 0
              ? `${(s.usage.unpriced_tokens / 1000).toFixed(0)}k tokens on unpriced models not counted`
              : s.budget.budget_eur
                ? `of €${s.budget.budget_eur.toFixed(2)} budget`
                : "no budget set"
          }
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className={cn(w.enforced && !w.open && "border-amber-500/60")}>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ClockIcon className="size-4" />
              Testing window
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            {!w.configured ? (
              <p className="text-muted-foreground">No window configured: testing is always allowed.</p>
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <Badge variant={w.open ? "secondary" : "destructive"}>{w.open ? "Open" : "Closed"}</Badge>
                  <Badge variant={w.enforced ? "default" : "outline"}>{w.enforced ? "Enforced" : "Not enforced"}</Badge>
                </div>
                {w.reason && <p className="text-muted-foreground">{w.reason}</p>}
                {w.next_change && (
                  <p>
                    {w.open ? "Closes" : "Opens"} {new Date(w.next_change).toLocaleString()}
                  </p>
                )}
                {!w.enforced && (
                  <p className="text-muted-foreground text-xs">
                    Turn on enforcement in Edit to pause tasks and block targets outside the window.
                  </p>
                )}
              </>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Budget</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {s.budget.budget_eur ? (
              <>
                <Progress value={Math.min(100, usedPct)} className={cn(usedPct >= 100 && "[&>div]:bg-destructive")} />
                <p>
                  €{(s.usage?.cost_eur ?? 0).toFixed(2)} of €{s.budget.budget_eur.toFixed(2)} ({usedPct.toFixed(0)}%)
                </p>
                <p className="text-muted-foreground text-xs">
                  At 100%: {s.budget.action === "pause" ? "tasks are paused" : "an alert is sent"}.
                </p>
              </>
            ) : (
              <p className="text-muted-foreground">No budget set.</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Retests</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            {s.findings.total === 0 ? (
              <p className="text-muted-foreground">No findings yet.</p>
            ) : (
              <>
                <p>Reproduced: {retests.reproduced ?? 0}</p>
                <p>Fixed: {retests.fixed ?? 0}</p>
                <p>Inconclusive: {retests.inconclusive ?? 0}</p>
                <p className="text-muted-foreground">Not retested: {retests.none ?? 0}</p>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Findings by severity</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {SEVERITIES.map((k) => (
              <div key={k} className="flex items-center gap-2 text-sm">
                <span className="w-16 capitalize">{k}</span>
                <div className="h-3 flex-1 rounded bg-muted">
                  <div
                    className={cn("h-3 rounded", SEV_BAR[k])}
                    style={{ width: `${((sev[k] ?? 0) / maxSev) * 100}%` }}
                  />
                </div>
                <span className="w-6 text-right tabular-nums">{sev[k] ?? 0}</span>
              </div>
            ))}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Recent activity</CardTitle>
          </CardHeader>
          <CardContent>
            {s.events.length === 0 ? (
              <p className="text-muted-foreground text-sm">Nothing yet.</p>
            ) : (
              <ul className="space-y-1.5 text-sm">
                {s.events.slice(0, 6).map((e) => (
                  <li key={e.id} className="flex justify-between gap-2">
                    <span className="truncate">
                      {e.kind.replaceAll("_", " ")}
                      {describeEvent(e) && <span className="text-muted-foreground"> · {describeEvent(e)}</span>}
                    </span>
                    <span className="shrink-0 text-muted-foreground text-xs">
                      {new Date(e.ts).toLocaleDateString()}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function EngagementDetailInner() {
  const router = useRouter();
  const id = Number(useSearchParams().get("id") ?? 0);
  const [summary, setSummary] = React.useState<EngagementSummary | null>(null);
  const [error, setError] = React.useState("");
  const [editing, setEditing] = React.useState(false);
  const [tab, setTab] = React.useState("overview");

  const load = React.useCallback(() => {
    if (!id) return;
    api
      .engagementSummary(id)
      .then((s) => {
        setSummary(s);
        setError("");
      })
      .catch((e: Error) => setError(e.message));
  }, [id]);

  React.useEffect(() => {
    load();
    const t = setInterval(load, 15000);
    return () => clearInterval(t);
  }, [load]);

  if (!id || error) {
    return (
      <div className="space-y-3 p-6">
        <Link href="/function/engagements" className="inline-flex items-center gap-1 text-sm hover:underline">
          <ArrowLeftIcon className="size-4" />
          Engagements
        </Link>
        <p className="text-muted-foreground text-sm">{error || "No engagement selected."}</p>
      </div>
    );
  }
  if (!summary) return <p className="p-6 text-muted-foreground text-sm">Loading…</p>;

  const e = summary.engagement;
  const status = ENGAGEMENT_STATUSES.find((s) => s.value === e.status)?.label ?? e.status;

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <Link
            href="/function/engagements"
            className="inline-flex items-center gap-1 text-muted-foreground text-xs hover:underline"
          >
            <ArrowLeftIcon className="size-3" />
            Engagements
          </Link>
          <h1 className="flex flex-wrap items-center gap-2 font-semibold text-xl">
            {e.name}
            <Badge variant="secondary">{status}</Badge>
          </h1>
          {e.company_name && <p className="text-muted-foreground text-sm">Client: {e.company_name}</p>}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            <PencilIcon className="size-3.5" />
            Edit
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="outline" size="sm">
                <Trash2Icon className="size-3.5" />
                Delete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete this engagement?</AlertDialogTitle>
                <AlertDialogDescription>
                  Its tasks and findings are kept; they are only detached, and the window, do-not-test entries and
                  budget stop applying to them.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={() =>
                    api
                      .deleteEngagement(id)
                      .then(() => {
                        toast.success("Engagement deleted");
                        router.push("/function/engagements");
                      })
                      .catch((err: Error) => toast.error(err.message))
                  }
                >
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="scope">Scope &amp; RoE</TabsTrigger>
          <TabsTrigger value="tasks">Tasks ({summary.tasks.length})</TabsTrigger>
          <TabsTrigger value="activity">Activity</TabsTrigger>
        </TabsList>
        <div className="mt-4">
          <TabsContent value="overview" className="mt-0">
            <Overview s={summary} />
          </TabsContent>
          <TabsContent value="scope" className="mt-0">
            <ScopeTab engagement={e} />
          </TabsContent>
          <TabsContent value="tasks" className="mt-0">
            <TasksTab engagementId={id} summary={summary} onChanged={load} />
          </TabsContent>
          <TabsContent value="activity" className="mt-0">
            <ActivityTab engagementId={id} />
          </TabsContent>
        </div>
      </Tabs>

      <EngagementDialog open={editing} onOpenChange={setEditing} engagement={e} onSaved={load} />
    </div>
  );
}

// useSearchParams must sit under a Suspense boundary for static export.
export default function EngagementDetailPage() {
  return (
    <React.Suspense fallback={null}>
      <EngagementDetailInner />
    </React.Suspense>
  );
}
