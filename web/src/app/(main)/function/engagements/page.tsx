"use client";

import * as React from "react";

import Link from "next/link";

import { BriefcaseBusinessIcon, PlusIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api";
import type { Engagement } from "@/lib/types";

import { ENGAGEMENT_STATUSES, EngagementDialog } from "./_components/engagement-form";

const statusLabel = (s: string) => ENGAGEMENT_STATUSES.find((x) => x.value === s)?.label ?? s;

export default function EngagementsPage() {
  const [list, setList] = React.useState<Engagement[] | null>(null);
  const [creating, setCreating] = React.useState(false);

  const load = React.useCallback(() => {
    api
      .engagements()
      .then(setList)
      .catch(() => setList([]));
  }, []);
  React.useEffect(load, [load]);

  return (
    <div className="space-y-4 p-4 md:p-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="flex items-center gap-2 font-semibold text-xl">
            <BriefcaseBusinessIcon className="size-5" />
            Engagements
          </h1>
          <p className="text-muted-foreground text-sm">
            Scope, rules of engagement, testing window and budget for a client assessment.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <PlusIcon className="size-4" />
          New engagement
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          {list === null ? (
            <p className="p-6 text-muted-foreground text-sm">Loading…</p>
          ) : list.length === 0 ? (
            <div className="p-10 text-center text-muted-foreground text-sm">
              No engagements yet. Create one to group tasks under a scope, a testing window and a budget.
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Client</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Tasks</TableHead>
                  <TableHead>Window</TableHead>
                  <TableHead className="text-right">Budget</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="font-medium">
                      <Link href={`/function/engagements/detail?id=${e.id}`} className="hover:underline">
                        {e.name}
                      </Link>
                    </TableCell>
                    <TableCell>{e.company_name || "–"}</TableCell>
                    <TableCell>
                      <Badge variant={e.status === "closed" ? "outline" : "secondary"}>{statusLabel(e.status)}</Badge>
                    </TableCell>
                    <TableCell className="text-right">{e.task_count}</TableCell>
                    <TableCell>
                      {e.window_enforce ? (
                        <Badge>Enforced</Badge>
                      ) : (
                        <span className="text-muted-foreground text-xs">not enforced</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      {e.budget_eur != null ? `€${e.budget_eur.toFixed(2)}` : "–"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <EngagementDialog open={creating} onOpenChange={setCreating} onSaved={load} />
    </div>
  );
}
