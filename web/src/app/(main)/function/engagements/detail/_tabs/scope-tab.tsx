"use client";

import * as React from "react";

import { ShieldBanIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import type { Engagement, EngagementScopeRow } from "@/lib/types";

export function ScopeTab({ engagement }: { engagement: Engagement }) {
  const [rows, setRows] = React.useState<EngagementScopeRow[]>([]);
  const [kind, setKind] = React.useState("domain");
  const [value, setValue] = React.useState("");
  const [excluded, setExcluded] = React.useState(false);
  const [reason, setReason] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  const load = React.useCallback(() => {
    api
      .engagementScope(engagement.id)
      .then(setRows)
      .catch(() => setRows([]));
  }, [engagement.id]);
  React.useEffect(load, [load]);

  const add = async () => {
    if (!value.trim()) return;
    setBusy(true);
    try {
      const r = await api.addEngagementScope(engagement.id, {
        kind,
        value: value.trim(),
        excluded,
        reason: reason.trim(),
      });
      toast.success(r.added ? `Added ${r.value}` : `${r.value} is already listed`);
      setValue("");
      setReason("");
      load();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const inScope = rows.filter((r) => !r.excluded);
  const out = rows.filter((r) => r.excluded);

  const list = (items: EngagementScopeRow[], empty: string) =>
    items.length === 0 ? (
      <p className="text-muted-foreground text-sm">{empty}</p>
    ) : (
      <ul className="divide-y">
        {items.map((r) => (
          <li key={r.id} className="flex items-center justify-between gap-2 py-1.5 text-sm">
            <span className="flex min-w-0 items-center gap-2">
              <Badge variant="outline">{r.kind}</Badge>
              <span className="truncate font-mono text-xs">{r.value}</span>
              {r.reason && <span className="truncate text-muted-foreground text-xs">· {r.reason}</span>}
            </span>
            <Button
              size="icon"
              variant="ghost"
              className="size-7"
              aria-label={`Remove ${r.value}`}
              onClick={() =>
                api
                  .deleteEngagementScope(engagement.id, r.id)
                  .then(load)
                  .catch((e: Error) => toast.error(e.message))
              }
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          </li>
        ))}
      </ul>
    );

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle className="text-base">Rules of engagement</CardTitle>
          <CardDescription>Edit them with the Edit button above.</CardDescription>
        </CardHeader>
        <CardContent>
          {engagement.roe ? (
            <p className="whitespace-pre-wrap text-sm">{engagement.roe}</p>
          ) : (
            <p className="text-muted-foreground text-sm">No rules of engagement written down yet.</p>
          )}
        </CardContent>
      </Card>

      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle className="text-base">Add a scope entry</CardTitle>
          <CardDescription>
            Do-not-test entries are enforced: every task of this engagement is blocked from reaching them (a domain also
            blocks its subdomains). In-scope entries document what may be tested; they are not an allowlist the gate
            enforces.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-2">
          <Select value={kind} onValueChange={setKind}>
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="domain">Domain</SelectItem>
                <SelectItem value="ip">IP</SelectItem>
                <SelectItem value="cidr">CIDR</SelectItem>
                <SelectItem value="keyword" disabled={excluded}>
                  Keyword
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <Input
            className="min-w-48 flex-1"
            placeholder="example.com, 10.0.0.0/24…"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void add()}
          />
          <Input
            className="w-48"
            placeholder="Reason (optional)"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={excluded}
              onCheckedChange={(v) => {
                setExcluded(v);
                if (v && kind === "keyword") setKind("domain");
              }}
            />
            Do not test
          </label>
          <Button disabled={busy || !value.trim()} onClick={() => void add()}>
            Add
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">In scope</CardTitle>
        </CardHeader>
        <CardContent>{list(inScope, "Nothing listed.")}</CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldBanIcon className="size-4" />
            Do not test
          </CardTitle>
        </CardHeader>
        <CardContent>{list(out, "Nothing excluded.")}</CardContent>
      </Card>
    </div>
  );
}
