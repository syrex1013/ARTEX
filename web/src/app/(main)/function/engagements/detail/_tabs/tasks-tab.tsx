"use client";

import * as React from "react";

import Link from "next/link";

import { toast } from "sonner";

import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import type { EngagementSummary, Task } from "@/lib/types";

export function TasksTab({
  engagementId,
  summary,
  onChanged,
}: {
  engagementId: number;
  summary: EngagementSummary;
  onChanged: () => void;
}) {
  const [all, setAll] = React.useState<Task[]>([]);
  const [pick, setPick] = React.useState("");
  React.useEffect(() => {
    api
      .tasks()
      .then((r) => setAll(r.tasks))
      .catch(() => setAll([]));
  }, []);

  const attachable = all.filter((t) => !t.engagement_id && t.status !== "done" && t.status !== "failed");
  const attach = async () => {
    if (!pick) return;
    try {
      await api.attachEngagementTask(engagementId, Number(pick));
      toast.success("Task attached");
      setPick("");
      onChanged();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Select value={pick} onValueChange={setPick}>
          <SelectTrigger className="w-80">
            <SelectValue placeholder="Attach an existing task…" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {attachable.map((t) => (
                <SelectItem key={t.id} value={String(t.id)}>
                  #{t.id} {t.name || t.description}
                </SelectItem>
              ))}
              {attachable.length === 0 && (
                <div className="px-2 py-1.5 text-muted-foreground text-xs">No unassigned active tasks.</div>
              )}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button disabled={!pick} onClick={() => void attach()}>
          Attach
        </Button>
        <span className="text-muted-foreground text-xs">
          New tasks can be assigned to this engagement in the task creation form.
        </span>
      </div>
      <Card>
        <CardContent className="p-0">
          {summary.tasks.length === 0 ? (
            <p className="p-6 text-muted-foreground text-sm">No tasks in this engagement yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Task</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="w-24" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {summary.tasks.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>
                      <Link href={`/function/tasks/detail?id=${t.id}`} className="font-medium hover:underline">
                        #{t.id} {t.name || t.description}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StatusBadge domain="task" value={t.paused ? "paused" : t.queued ? "queued" : t.status} />
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() =>
                          api
                            .detachEngagementTask(engagementId, t.id)
                            .then(onChanged)
                            .catch((e: Error) => toast.error(e.message))
                        }
                      >
                        Detach
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
