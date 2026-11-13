"use client";

import { useEffect, useState } from "react";

import Link from "next/link";

import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useCurrentUser } from "@/hooks/use-current-user";
import { http } from "@/lib/api";
import { auth } from "@/lib/auth";
import { collaborationTime, mutation } from "@/lib/collaboration";

interface Preferences {
  timezone: string;
  send_key: string;
  theme: string;
  density: string;
}
interface Session {
  id: string;
  created_at: string;
  expires_at: string;
  user_agent: string;
  current: boolean;
}
interface Account {
  id: number;
  username: string;
  display_name: string;
  enabled: boolean;
  admin: boolean;
}
interface Limits {
  session_days: number;
  report_concurrency: number;
  generation_timeout_seconds: number;
  upload_limit_mib: number;
  generation_token_budget: number;
}
export function AccountSettings() {
  const user = useCurrentUser();
  const { setTheme } = useTheme();
  const [name, setName] = useState("");
  const [preferences, setPreferences] = useState<Preferences>({
    timezone: "UTC",
    send_key: "enter",
    theme: "system",
    density: "comfortable",
  });
  const [sessions, setSessions] = useState<Session[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [limits, setLimits] = useState<Limits | null>(null);
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [oldPassword, setOldPassword] = useState("");
  const [nextPassword, setNextPassword] = useState("");
  const [resetID, setResetID] = useState("");
  const [resetPassword, setResetPassword] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(() => {
    const controller = new AbortController();
    void Promise.allSettled([
      http<{ display_name: string; preferences: Partial<Preferences> }>("/auth/me", { signal: controller.signal }).then(
        (me) => {
          if (!controller.signal.aborted) {
            setName(me.display_name);
            setPreferences((p) => ({ ...p, ...me.preferences }));
          }
        },
      ),
      http<Session[]>("/auth/sessions", { signal: controller.signal }).then((s) => {
        if (!controller.signal.aborted) setSessions(s);
      }),
    ]).then((results) => {
      if (!controller.signal.aborted) {
        const failed = results.find((r) => r.status === "rejected");
        if (failed?.status === "rejected") setError(String(failed.reason));
      }
    });
    return () => controller.abort();
  }, [revision]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(() => {
    if (user.role !== "admin") return;
    const controller = new AbortController();
    void Promise.allSettled([
      http<Account[]>("/accounts", { signal: controller.signal }).then(setAccounts),
      http<Limits>("/collaboration-limits", { signal: controller.signal }).then(setLimits),
    ]).then((results) => {
      if (!controller.signal.aborted) {
        const failed = results.find((r) => r.status === "rejected");
        if (failed?.status === "rejected") setError(String(failed.reason));
      }
    });
    return () => controller.abort();
  }, [user.role, revision]);
  async function act(operation: () => Promise<unknown>, reload = true) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await operation();
      setNotice("Saved");
      if (reload) setRevision((r) => r + 1);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <h1 className="font-semibold text-2xl">Account and settings</h1>
      {error && (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Account and appearance</CardTitle>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                void act(async () => {
                  await mutation("/auth/me", "PATCH", { display_name: name, preferences });
                  setTheme(preferences.theme);
                  localStorage.setItem("artex_timezone", preferences.timezone);
                  document.documentElement.dataset.density = preferences.density;
                });
              }}
            >
              <label className="block text-sm" htmlFor="account-settings-1">
                Display name
                <Input
                  id="account-settings-1"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={200}
                />
              </label>
              <label className="block text-sm" htmlFor="account-settings-2">
                Timezone
                <Input
                  id="account-settings-2"
                  value={preferences.timezone}
                  onChange={(e) => setPreferences({ ...preferences, timezone: e.target.value })}
                  placeholder="Europe/Warsaw"
                />
              </label>
              {(
                [
                  ["send_key", ["enter", "mod-enter"]],
                  ["theme", ["system", "light", "dark"]],
                  ["density", ["comfortable", "compact"]],
                ] as const
              ).map(([key, options]) => (
                <label key={key} className="block text-sm">
                  {key.replaceAll("_", " ")}
                  <select
                    className="ml-3 rounded-md border bg-background p-2"
                    value={preferences[key]}
                    onChange={(e) => setPreferences({ ...preferences, [key]: e.target.value })}
                  >
                    {options.map((option) => (
                      <option key={option}>{option}</option>
                    ))}
                  </select>
                </label>
              ))}
              <Button disabled={busy}>Save account</Button>
              <p className="text-muted-foreground text-xs">Presentation preferences affect your account only.</p>
            </form>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Password</CardTitle>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                void act(async () => {
                  await mutation("/auth/change-password", "POST", {
                    old_password: oldPassword,
                    new_password: nextPassword,
                  });
                  auth.clearToken();
                  window.location.href = "/login";
                }, false);
              }}
            >
              <label className="block text-sm" htmlFor="account-settings-3">
                Current password
                <Input
                  id="account-settings-3"
                  type="password"
                  autoComplete="current-password"
                  value={oldPassword}
                  onChange={(e) => setOldPassword(e.target.value)}
                  required
                />
              </label>
              <label className="block text-sm" htmlFor="account-settings-4">
                New password
                <Input
                  id="account-settings-4"
                  type="password"
                  autoComplete="new-password"
                  minLength={12}
                  value={nextPassword}
                  onChange={(e) => setNextPassword(e.target.value)}
                  required
                />
              </label>
              <Button disabled={busy}>Change password and sign out all sessions</Button>
            </form>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Active sessions</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {sessions.map((s) => (
              <div key={s.id} className="space-y-2 border-b pb-3">
                <p className="break-all text-sm">
                  {s.current ? "Current session · " : ""}
                  {s.user_agent}
                </p>
                <p className="text-muted-foreground text-xs">Expires {collaborationTime(s.expires_at)}</p>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void act(async () => {
                      await mutation(`/auth/sessions/${s.id}`, "DELETE");
                      if (s.current) {
                        auth.clearToken();
                        window.location.href = "/login";
                      }
                    })
                  }
                >
                  Revoke session
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Engagement, AI and notifications</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm">
              Membership, reporting metadata, dates and notification preferences belong to each engagement.
            </p>
            <Link className="underline" href="/engagements">
              Open engagement settings
            </Link>
            {user.role === "admin" && (
              <div className="flex flex-wrap gap-3">
                <Link className="underline" href="/system/llm">
                  AI providers and budgets
                </Link>
                <Link className="underline" href="/system/notify">
                  Notification delivery
                </Link>
                <Link className="underline" href="/system/settings">
                  Execution controls
                </Link>
              </div>
            )}
          </CardContent>
        </Card>
        {user.role === "admin" && (
          <>
            <Card>
              <CardHeader>
                <CardTitle>Administration · local accounts</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <form
                  className="space-y-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void act(async () => {
                      await mutation("/accounts", "POST", { username, display_name: displayName, password });
                      setPassword("");
                      setUsername("");
                      setDisplayName("");
                    });
                  }}
                >
                  <Input
                    aria-label="New username"
                    placeholder="Username"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    required
                  />
                  <Input
                    aria-label="New account display name"
                    placeholder="Display name"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    required
                  />
                  <Input
                    aria-label="Initial account password"
                    type="password"
                    autoComplete="new-password"
                    minLength={12}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                  />
                  <Button disabled={busy}>Create account</Button>
                </form>
                {accounts.map((a) => (
                  <div key={a.id} className="flex items-center justify-between gap-3 border-b pb-2">
                    <span>
                      {a.display_name} · @{a.username} · #{a.id}
                      {a.admin && " · administrator"}
                    </span>
                    {String(a.id) !== user.id && (
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={() => void act(() => mutation(`/accounts/${a.id}`, "PATCH", { enabled: !a.enabled }))}
                      >
                        {a.enabled ? "Disable" : "Enable"}
                      </Button>
                    )}
                  </div>
                ))}
                <form
                  className="space-y-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void act(async () => {
                      await mutation(`/accounts/${resetID}/reset-password`, "POST", { new_password: resetPassword });
                      setResetPassword("");
                    });
                  }}
                >
                  <Input
                    aria-label="Account ID to reset"
                    placeholder="Account ID"
                    type="number"
                    min={1}
                    required
                    value={resetID}
                    onChange={(e) => setResetID(e.target.value)}
                  />
                  <Input
                    aria-label="Reset password"
                    placeholder="New password"
                    type="password"
                    autoComplete="new-password"
                    minLength={12}
                    required
                    value={resetPassword}
                    onChange={(e) => setResetPassword(e.target.value)}
                  />
                  <Button variant="outline" disabled={busy}>
                    Reset password and revoke sessions
                  </Button>
                </form>
              </CardContent>
            </Card>
            {limits && (
              <Card>
                <CardHeader>
                  <CardTitle>Administration · effective limits</CardTitle>
                </CardHeader>
                <CardContent>
                  <form
                    className="space-y-3"
                    onSubmit={(e) => {
                      e.preventDefault();
                      void act(() => mutation("/collaboration-limits", "PUT", limits));
                    }}
                  >
                    {(Object.keys(limits) as (keyof Limits)[]).map((key) => (
                      <label key={key} className="block text-sm" htmlFor={`limit-${key}`}>
                        {key.replaceAll("_", " ")}
                        <Input
                          id={`limit-${key}`}
                          type="number"
                          min={1}
                          required
                          value={limits[key]}
                          onChange={(e) => setLimits({ ...limits, [key]: Number(e.target.value) })}
                        />
                      </label>
                    ))}
                    <Button disabled={busy}>Save system limits</Button>
                    <p className="text-muted-foreground text-xs">
                      Applies to future sessions and generations. Existing sessions keep their expiry. Automatic
                      retention deletion remains disabled.
                    </p>
                  </form>
                </CardContent>
              </Card>
            )}
          </>
        )}
      </div>
    </div>
  );
}
