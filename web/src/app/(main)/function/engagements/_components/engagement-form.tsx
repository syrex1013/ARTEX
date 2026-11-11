"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { Company, DailyWindow, Engagement, EngagementInput, EngagementStatus } from "@/lib/types";

export const ENGAGEMENT_STATUSES: { value: EngagementStatus; label: string }[] = [
  { value: "scoping", label: "Scoping" },
  { value: "testing", label: "Testing" },
  { value: "reporting", label: "Reporting" },
  { value: "retest", label: "Retest" },
  { value: "closed", label: "Closed" },
];

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

// RFC3339 <-> <input type="datetime-local"> (browser-local wall clock).
const toLocalInput = (iso?: string | null) => {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
};
const fromLocalInput = (v: string) => (v ? new Date(v).toISOString() : "");

/** Create or edit an engagement. Pass `engagement` to edit. */
export function EngagementDialog({
  open,
  onOpenChange,
  engagement,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  engagement?: Engagement;
  onSaved: (e: Engagement) => void;
}) {
  const [companies, setCompanies] = React.useState<Company[]>([]);
  const [name, setName] = React.useState("");
  const [companyId, setCompanyId] = React.useState<string>("none");
  const [status, setStatus] = React.useState<EngagementStatus>("scoping");
  const [roe, setRoe] = React.useState("");
  const [tz, setTz] = React.useState("");
  const [start, setStart] = React.useState("");
  const [end, setEnd] = React.useState("");
  const [slots, setSlots] = React.useState<DailyWindow[]>([]);
  const [enforce, setEnforce] = React.useState(false);
  const [budget, setBudget] = React.useState("");
  const [alerts, setAlerts] = React.useState("80, 100");
  const [action, setAction] = React.useState<"alert" | "pause">("alert");
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    api
      .companies()
      .then(setCompanies)
      .catch(() => setCompanies([]));
    const e = engagement;
    setName(e?.name ?? "");
    setCompanyId(e?.company_id ? String(e.company_id) : "none");
    setStatus(e?.status ?? "scoping");
    setRoe(e?.roe ?? "");
    setTz(e?.window_tz ?? "");
    setStart(toLocalInput(e?.window_start));
    setEnd(toLocalInput(e?.window_end));
    setSlots(e?.daily_windows ?? []);
    setEnforce(e?.window_enforce ?? false);
    setBudget(e?.budget_eur != null ? String(e.budget_eur) : "");
    setAlerts((e?.budget_alert_pct ?? [80, 100]).join(", "));
    setAction(e?.budget_action ?? "alert");
  }, [open, engagement]);

  const save = async () => {
    if (!name.trim()) {
      toast.error("Give the engagement a name");
      return;
    }
    const pct = alerts
      .split(/[,\s]+/)
      .filter(Boolean)
      .map(Number);
    if (pct.some((n) => !Number.isFinite(n))) {
      toast.error("Alert thresholds must be numbers, e.g. 80, 100");
      return;
    }
    const input: EngagementInput = {
      name: name.trim(),
      status,
      roe,
      window_tz: tz.trim(),
      window_start: fromLocalInput(start),
      window_end: fromLocalInput(end),
      daily_windows: slots,
      window_enforce: enforce,
      budget_alert_pct: pct,
      budget_action: action,
    };
    if (companyId === "none") input.clear_company = true;
    else input.company_id = Number(companyId);
    if (budget.trim() === "") input.clear_budget = true;
    else input.budget_eur = Number(budget);
    setSaving(true);
    try {
      const saved = engagement ? await api.updateEngagement(engagement.id, input) : await api.createEngagement(input);
      toast.success(engagement ? "Engagement updated" : "Engagement created");
      onSaved(saved);
      onOpenChange(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const setSlot = (i: number, patch: Partial<DailyWindow>) =>
    setSlots((s) => s.map((x, j) => (j === i ? { ...x, ...patch } : x)));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{engagement ? "Edit engagement" : "New engagement"}</DialogTitle>
          <DialogDescription>
            An engagement groups tasks under one scope, rules of engagement, testing window and budget.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 text-sm">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="eng-name">Name</Label>
              <Input
                id="eng-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Acme external assessment"
              />
            </div>
            <div className="space-y-1.5">
              <Label>Client (company)</Label>
              <Select value={companyId} onValueChange={setCompanyId}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="none">No company</SelectItem>
                    {companies.map((c) => (
                      <SelectItem key={c.id} value={String(c.id)}>
                        {c.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>Status</Label>
              <Select value={status} onValueChange={(v) => setStatus(v as EngagementStatus)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {ENGAGEMENT_STATUSES.map((s) => (
                      <SelectItem key={s.value} value={s.value}>
                        {s.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="eng-roe">Rules of engagement</Label>
            <Textarea
              id="eng-roe"
              rows={4}
              value={roe}
              onChange={(e) => setRoe(e.target.value)}
              placeholder="What is allowed, what is not, who to call on an incident…"
            />
          </div>

          <fieldset className="space-y-3 rounded-lg border p-3">
            <legend className="px-1 font-medium">Testing window</legend>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-1.5">
                <Label htmlFor="eng-tz">Time zone</Label>
                <Input
                  id="eng-tz"
                  value={tz}
                  onChange={(e) => setTz(e.target.value)}
                  placeholder="UTC, Europe/Berlin…"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="eng-start">Not before</Label>
                <Input id="eng-start" type="datetime-local" value={start} onChange={(e) => setStart(e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="eng-end">Not after</Label>
                <Input id="eng-end" type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} />
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              The period above is in your browser&apos;s time zone. Daily slots use the engagement time zone; an end
              before the start runs past midnight. With no slots the whole period is open.
            </p>
            <div className="space-y-2">
              {slots.map((s, i) => (
                // biome-ignore lint/suspicious/noArrayIndexKey: slots have no id; the list is edited in place
                <div key={i} className="flex flex-wrap items-center gap-2 rounded-md border p-2">
                  <div className="flex gap-1">
                    {WEEKDAYS.map((d, n) => {
                      const day = n + 1;
                      const on = s.days.length === 0 || s.days.includes(day);
                      return (
                        <button
                          key={d}
                          type="button"
                          className={`rounded px-1.5 py-0.5 text-xs ${on ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"}`}
                          onClick={() => {
                            const all = s.days.length === 0 ? [1, 2, 3, 4, 5, 6, 7] : s.days;
                            const next = all.includes(day) ? all.filter((x) => x !== day) : [...all, day].sort();
                            setSlot(i, { days: next.length === 7 ? [] : next });
                          }}
                        >
                          {d}
                        </button>
                      );
                    })}
                  </div>
                  <Input
                    className="h-8 w-24"
                    type="time"
                    value={s.start}
                    onChange={(e) => setSlot(i, { start: e.target.value })}
                  />
                  <span>–</span>
                  <Input
                    className="h-8 w-24"
                    type="time"
                    value={s.end}
                    onChange={(e) => setSlot(i, { end: e.target.value })}
                  />
                  <Button size="sm" variant="ghost" onClick={() => setSlots((x) => x.filter((_, j) => j !== i))}>
                    Remove
                  </Button>
                </div>
              ))}
              <Button
                size="sm"
                variant="outline"
                onClick={() => setSlots((x) => [...x, { days: [], start: "09:00", end: "17:00" }])}
              >
                Add daily slot
              </Button>
            </div>
            <div className="flex items-center gap-2">
              <Switch id="eng-enforce" checked={enforce} onCheckedChange={setEnforce} />
              <Label htmlFor="eng-enforce" className="font-normal">
                Enforce: pause the engagement&apos;s tasks and block all network targets outside the window
              </Label>
            </div>
          </fieldset>

          <fieldset className="space-y-3 rounded-lg border p-3">
            <legend className="px-1 font-medium">Budget</legend>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-1.5">
                <Label htmlFor="eng-budget">Budget (EUR)</Label>
                <Input
                  id="eng-budget"
                  inputMode="decimal"
                  value={budget}
                  onChange={(e) => setBudget(e.target.value)}
                  placeholder="none"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="eng-alerts">Alert at (%)</Label>
                <Input id="eng-alerts" value={alerts} onChange={(e) => setAlerts(e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label>At 100 %</Label>
                <Select value={action} onValueChange={(v) => setAction(v as "alert" | "pause")}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="alert">Alert only</SelectItem>
                      <SelectItem value="pause">Pause the tasks</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              Cost is computed from the prices set on LLM profiles; tokens of unpriced models are shown separately and
              not counted. Alerts go to your enabled notification channels.
            </p>
          </fieldset>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
