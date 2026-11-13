"use client";

import { useEffect, useState } from "react";

import { type Engagement, useEngagement } from "@/components/engagement-provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useCurrentUser } from "@/hooks/use-current-user";
import { http } from "@/lib/api";
import { engagementPath, mutation } from "@/lib/collaboration";

interface Member {
  user_id: number;
  username: string;
  display_name: string;
  role: string;
  enabled: boolean;
}
export function EngagementSettings({ id, editable }: { id: number; editable: boolean }) {
  const { engagement, reload } = useEngagement();
  const user = useCurrentUser();
  const [form, setForm] = useState<Engagement | undefined>(engagement);
  const [members, setMembers] = useState<Member[]>([]);
  const [uid, setUid] = useState("");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revision, setRevision] = useState(0);
  const [taskID, setTaskID] = useState("");
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(() => {
    const controller = new AbortController();
    void http<Member[]>(`${engagementPath(id)}/members`, { signal: controller.signal })
      .then(setMembers)
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => controller.abort();
  }, [id, revision]);
  async function act(operation: () => Promise<unknown>) {
    setError("");
    setNotice("");
    try {
      await operation();
      setRevision((r) => r + 1);
      reload();
      setNotice("Saved. Membership changes apply immediately; report defaults apply to future jobs.");
    } catch (e) {
      setError((e as Error).message);
    }
  }
  if (!form) return null;
  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Engagement details</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              void act(() => mutation(engagementPath(id), "PATCH", form));
            }}
          >
            <label className="block text-sm" htmlFor="engagement-settings-1">
              Name
              <Input
                id="engagement-settings-1"
                disabled={!editable}
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </label>
            <label className="block text-sm" htmlFor="engagement-settings-2">
              Description
              <Textarea
                id="engagement-settings-2"
                disabled={!editable}
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </label>
            <label className="block text-sm">
              Status
              <select
                disabled={!editable}
                className="block rounded-md border bg-background p-2"
                value={form.status}
                onChange={(e) => setForm({ ...form, status: e.target.value })}
              >
                {["planned", "active", "completed", "archived"].map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </label>
            <div className="grid grid-cols-2 gap-3">
              <label className="text-sm" htmlFor="engagement-settings-3">
                Start date
                <Input
                  id="engagement-settings-3"
                  disabled={!editable}
                  type="date"
                  value={form.starts_on?.slice(0, 10) ?? ""}
                  onChange={(e) => setForm({ ...form, starts_on: e.target.value || null })}
                />
              </label>
              <label className="text-sm" htmlFor="engagement-settings-4">
                End date
                <Input
                  id="engagement-settings-4"
                  disabled={!editable}
                  type="date"
                  value={form.ends_on?.slice(0, 10) ?? ""}
                  onChange={(e) => setForm({ ...form, ends_on: e.target.value || null })}
                />
              </label>
            </div>
            <label className="block text-sm" htmlFor="engagement-settings-5">
              Default report model profile ID (0 uses global default)
              <Input
                id="engagement-settings-5"
                disabled={!editable}
                type="number"
                min={0}
                value={form.settings.default_report_model ?? 0}
                onChange={(e) =>
                  setForm({ ...form, settings: { ...form.settings, default_report_model: Number(e.target.value) } })
                }
              />
            </label>
            <label className="block text-sm" htmlFor="engagement-settings-6">
              Reporting contact
              <Input
                id="engagement-settings-6"
                disabled={!editable}
                value={form.settings.reporting_metadata?.contact ?? ""}
                onChange={(e) =>
                  setForm({
                    ...form,
                    settings: {
                      ...form.settings,
                      reporting_metadata: { ...form.settings.reporting_metadata, contact: e.target.value },
                    },
                  })
                }
              />
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                disabled={!editable}
                checked={form.settings.notifications ?? false}
                onChange={(e) => setForm({ ...form, settings: { ...form.settings, notifications: e.target.checked } })}
              />
              Engagement notifications preference
            </label>
            <p className="text-muted-foreground text-xs">
              Automatic retention deletion is disabled. Existing execution and provider controls remain under
              Administration.
            </p>
            {editable && <Button>Save engagement</Button>}
            {error && (
              <p role="alert" className="text-destructive">
                {error}
              </p>
            )}
            {notice && <p role="status">{notice}</p>}
          </form>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Members and scope</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {members.map((m) => (
            <div key={m.user_id} className="flex flex-wrap items-center justify-between gap-2 border-b pb-2">
              <div>
                <p>
                  {m.display_name}{" "}
                  <span className="text-muted-foreground text-xs">
                    @{m.username} · #{m.user_id}
                  </span>
                </p>
                <p className="text-xs">
                  {m.role}
                  {!m.enabled && " · disabled"}
                </p>
              </div>
              {editable && String(m.user_id) !== user.id && (
                <Button
                  variant="outline"
                  onClick={() => void act(() => mutation(`${engagementPath(id)}/members/${m.user_id}`, "DELETE"))}
                >
                  Remove
                </Button>
              )}
            </div>
          ))}
          {editable && (
            <form
              className="space-y-2"
              onSubmit={(e) => {
                e.preventDefault();
                void act(() => mutation(`${engagementPath(id)}/members/${uid}`, "PUT", { role }));
              }}
            >
              <Input
                aria-label="Member account ID"
                placeholder="Account ID from administrator"
                type="number"
                min={1}
                required
                value={uid}
                onChange={(e) => setUid(e.target.value)}
              />
              <select
                aria-label="Member role"
                className="rounded-md border bg-background p-2"
                value={role}
                onChange={(e) => setRole(e.target.value)}
              >
                {["viewer", "operator", "manager"].map((r) => (
                  <option key={r}>{r}</option>
                ))}
              </select>
              <Button>Add / change member</Button>
            </form>
          )}
          {user.role === "admin" && (
            <form
              className="space-y-2"
              onSubmit={(e) => {
                e.preventDefault();
                void act(() => mutation(`${engagementPath(id)}/tasks/${taskID}`, "PUT"));
              }}
            >
              <Input
                aria-label="Task ID to assign"
                placeholder="Stopped task ID"
                type="number"
                min={1}
                required
                value={taskID}
                onChange={(e) => setTaskID(e.target.value)}
              />
              <Button variant="outline">Assign task to engagement</Button>
              <p className="text-muted-foreground text-xs">Only stopped tasks without source relations can move.</p>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
