"use client";

import * as React from "react";

import { Card, CardContent } from "@/components/ui/card";
import { api } from "@/lib/api";
import type { EngagementEvent } from "@/lib/types";

const LABELS: Record<string, string> = {
  created: "Engagement created",
  status_changed: "Status changed",
  window_changed: "Testing window changed",
  window_open: "Testing window opened",
  window_closed: "Testing window closed",
  budget_changed: "Budget changed",
  budget_threshold: "Budget threshold reached",
  budget_paused: "Tasks paused: budget used up",
  scope_added: "Scope entry added",
  scope_removed: "Scope entry removed",
  task_attached: "Task attached",
  task_detached: "Task detached",
};

export const describeEvent = (e: EngagementEvent) => {
  const d = e.detail ?? {};
  const bits: string[] = [];
  if (d.value) bits.push(`${d.excluded ? "do not test " : ""}${String(d.value)}`);
  if (d.from && d.to) bits.push(`${String(d.from)} → ${String(d.to)}`);
  if (d.task_id) bits.push(`task #${String(d.task_id)}`);
  if (d.threshold_pct)
    bits.push(
      `${String(d.threshold_pct)}% (€${Number(d.cost_eur ?? 0).toFixed(2)} of €${Number(d.budget_eur ?? 0).toFixed(2)})`,
    );
  if (d.reason) bits.push(String(d.reason));
  if (typeof d.tasks_paused === "number" && d.tasks_paused > 0) bits.push(`${d.tasks_paused} task(s) paused`);
  if (d.next_change) bits.push(`next change ${new Date(String(d.next_change)).toLocaleString()}`);
  return bits.join(" · ");
};

export function ActivityTab({ engagementId }: { engagementId: number }) {
  const [events, setEvents] = React.useState<EngagementEvent[] | null>(null);
  React.useEffect(() => {
    api
      .engagementEvents(engagementId, 200)
      .then(setEvents)
      .catch(() => setEvents([]));
  }, [engagementId]);

  return (
    <Card>
      <CardContent className="p-0">
        {events === null ? (
          <p className="p-6 text-muted-foreground text-sm">Loading…</p>
        ) : events.length === 0 ? (
          <p className="p-6 text-muted-foreground text-sm">No activity yet.</p>
        ) : (
          <ul className="divide-y">
            {events.map((e) => (
              <li key={e.id} className="flex flex-wrap items-baseline justify-between gap-2 px-4 py-2.5 text-sm">
                <span>
                  <span className="font-medium">{LABELS[e.kind] ?? e.kind}</span>
                  {describeEvent(e) && <span className="text-muted-foreground"> · {describeEvent(e)}</span>}
                </span>
                <span className="text-muted-foreground text-xs">
                  {e.actor} · {new Date(e.ts).toLocaleString()}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
