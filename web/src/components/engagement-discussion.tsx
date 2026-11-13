"use client";

import { useEffect, useRef, useState } from "react";

import { Markdown } from "@/components/markdown";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useCurrentUser } from "@/hooks/use-current-user";
import { http, sseUrl } from "@/lib/api";
import {
  collaborationTime,
  type DiscussionMessage,
  type DiscussionThread,
  downloadCollaboration,
  type EngagementTask,
  engagementPath,
  mutation,
  type ThreadAttachment,
} from "@/lib/collaboration";

export function EngagementDiscussion({ id }: { id: number }) {
  const base = engagementPath(id);
  const [threads, setThreads] = useState<DiscussionThread[]>([]);
  const [selected, setSelected] = useState<DiscussionThread | null>(null);
  const [title, setTitle] = useState("");
  const [kind, setKind] = useState("discussion");
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [page, setPage] = useState(1);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision refreshes persisted data after successful mutations.
  useEffect(() => {
    const controller = new AbortController();
    void http<DiscussionThread[]>(`${base}/threads?page=${page}&limit=25`, { signal: controller.signal })
      .then((list) => {
        setThreads(list);
        setError("");
      })
      .catch((e: Error) => {
        if (!controller.signal.aborted) setError(e.message);
      });
    return () => controller.abort();
  }, [base, page, revision]);
  async function create() {
    setBusy(true);
    try {
      await mutation(`${base}/threads`, "POST", { title, kind });
      setTitle("");
      setRevision((r) => r + 1);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="grid gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
      <Card>
        <CardHeader>
          <CardTitle>Discussion threads</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault();
              void create();
            }}
          >
            <Input
              aria-label="Thread title"
              placeholder="Thread title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              required
              maxLength={200}
            />
            <select
              aria-label="Thread visibility"
              className="w-full rounded-md border bg-background p-2 text-sm"
              value={kind}
              onChange={(e) => setKind(e.target.value)}
            >
              <option value="discussion">Shared human discussion</option>
              <option value="shared_ai">Shared AI conversation</option>
              <option value="private_ai">Private AI conversation</option>
            </select>
            <Button disabled={busy}>Create thread</Button>
          </form>
          {error && (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          )}
          {threads.length === 0 && <p className="text-muted-foreground text-sm">No threads on this page.</p>}
          {threads.map((t) => (
            <button
              type="button"
              key={t.id}
              className="block w-full rounded-md border p-3 text-left focus-visible:outline-2 focus-visible:outline-ring"
              aria-pressed={selected?.id === t.id}
              onClick={() => setSelected(t)}
            >
              <span className="block font-medium">{t.title}</span>
              <span className="text-muted-foreground text-xs">
                {t.kind} · {t.owner}
              </span>
            </button>
          ))}
          <div className="flex gap-2">
            <Button variant="outline" disabled={page === 1} onClick={() => setPage(page - 1)}>
              Previous
            </Button>
            <Button variant="outline" disabled={threads.length < 25} onClick={() => setPage(page + 1)}>
              Next
            </Button>
          </div>
        </CardContent>
      </Card>
      {selected ? (
        <ThreadMessages key={selected.id} base={base} thread={selected} />
      ) : (
        <p className="p-5 text-muted-foreground">Select a thread. Human messages never invoke AI.</p>
      )}
    </div>
  );
}
function ThreadMessages({ base, thread }: { base: string; thread: DiscussionThread }) {
  const path = `${base}/threads/${thread.id}`;
  const user = useCurrentUser();
  const [messages, setMessages] = useState<DiscussionMessage[]>([]);
  const [attachments, setAttachments] = useState<ThreadAttachment[]>([]);
  const [body, setBody] = useState("");
  const [tasks, setTasks] = useState<EngagementTask[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [attachment, setAttachment] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [connected, setConnected] = useState(false);
  const [page, setPage] = useState(1);
  const [sendKey, setSendKey] = useState("enter");
  const clientID = useRef<string | null>(null);
  const eventCursor = useRef(0);
  useEffect(() => {
    void http<{ preferences: { send_key?: string } }>("/auth/me")
      .then((u) => setSendKey(u.preferences.send_key ?? "enter"))
      .catch(() => undefined);
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setMessages([]);
    void Promise.allSettled([
      http<DiscussionMessage[]>(`${path}/messages?page=${page}&limit=50`, { signal: controller.signal }).then(
        (list) => {
          if (!controller.signal.aborted) setMessages(list);
        },
      ),
      http<ThreadAttachment[]>(`${path}/attachments`, { signal: controller.signal }).then((list) => {
        if (!controller.signal.aborted) setAttachments(list);
      }),
      http<{ items: EngagementTask[] }>(`${base}/tasks?limit=200`, { signal: controller.signal }).then((result) => {
        if (!controller.signal.aborted) setTasks(result.items);
      }),
    ]).then((results) => {
      if (!controller.signal.aborted) {
        const failed = results.find((r) => r.status === "rejected");
        if (failed?.status === "rejected") setError(String(failed.reason));
        setLoading(false);
      }
    });
    return () => controller.abort();
  }, [path, base, page]);
  useEffect(() => {
    let stream: EventSource | undefined;
    let stopped = false;
    const connect = () => {
      if (stopped || document.hidden || page !== 1) return;
      stream = new EventSource(sseUrl(`/api${path}/events?after=${eventCursor.current}`));
      stream.onopen = () => setConnected(true);
      stream.onerror = () => setConnected(false);
      stream.addEventListener("message", (event: MessageEvent<string>) => {
        eventCursor.current = Number(event.lastEventId) || eventCursor.current;
        const message = JSON.parse(event.data) as DiscussionMessage;
        setMessages((old) => {
          const map = new Map(old.map((m) => [m.id, m]));
          map.set(message.id, message);
          return [...map.values()].sort((a, b) => a.id - b.id).slice(-50);
        });
      });
    };
    const visibility = () => {
      stream?.close();
      setConnected(false);
      if (!document.hidden) connect();
    };
    connect();
    document.addEventListener("visibilitychange", visibility);
    return () => {
      stopped = true;
      stream?.close();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [path, page]);
  async function send(ai: boolean) {
    if (!body.trim()) return;
    setBusy(true);
    setError("");
    clientID.current ??= crypto.randomUUID();
    try {
      if (ai)
        await mutation(`${path}/ask`, "POST", { question: body, client_id: clientID.current, task_ids: selected });
      else
        await mutation(`${path}/messages`, "POST", {
          body,
          client_id: clientID.current,
          attachment_id: attachment || null,
        });
      setBody("");
      setAttachment("");
      clientID.current = null;
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function upload(file: File) {
    setBusy(true);
    try {
      const token = localStorage.getItem("artex_token");
      const response = await fetch(`/api${path}/attachments?name=${encodeURIComponent(file.name)}`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: file,
      });
      if (!response.ok) throw new Error(`Upload failed (${response.status})`);
      setAttachments(await http<ThreadAttachment[]>(`${path}/attachments`));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {thread.title}
          <Badge variant="outline">{thread.kind}</Badge>
          <span className="text-muted-foreground text-xs">{connected ? "Live" : "Disconnected / history"}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading && <p role="status">Loading messages…</p>}
        {error && (
          <p role="alert" className="text-destructive">
            {error}
          </p>
        )}
        <div className="max-h-[55vh] space-y-4 overflow-auto" aria-live="polite">
          {messages.map((m) => (
            <article key={m.id} className="space-y-2 rounded-md border p-3">
              <header className="flex flex-wrap items-center justify-between gap-2 text-muted-foreground text-xs">
                <span>
                  {m.kind === "ai" ? "AI · requested by " : ""}
                  {m.author}
                </span>
                <time dateTime={m.created_at}>{collaborationTime(m.created_at)}</time>
                <Badge variant="outline">{m.state}</Badge>
              </header>
              {m.body && <Markdown text={m.body} />} {m.error && <p className="text-destructive text-sm">{m.error}</p>}
              {m.attachment_id && (
                <Button
                  variant="link"
                  onClick={() =>
                    void downloadCollaboration(
                      `${path}/attachments/${m.attachment_id}`,
                      attachments.find((a) => a.id === m.attachment_id)?.name ?? "attachment",
                    ).catch((e: Error) => setError(e.message))
                  }
                >
                  Download attachment
                </Button>
              )}
              {m.kind === "ai" &&
                m.state === "running" &&
                (String(m.author_id) === user.id || user.role === "admin") && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      void mutation(`${path}/messages/${m.id}/cancel`, "POST").catch((e: Error) => setError(e.message))
                    }
                  >
                    Cancel AI
                  </Button>
                )}
            </article>
          ))}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" disabled={messages.length < 50} onClick={() => setPage(page + 1)}>
            Older messages
          </Button>
          <Button variant="outline" disabled={page === 1} onClick={() => setPage(page - 1)}>
            Newer messages
          </Button>
        </div>
        <Textarea
          aria-label="Message"
          placeholder="Write a message. Ask AI is always explicit."
          value={body}
          onChange={(e) => {
            setBody(e.target.value);
            clientID.current = null;
          }}
          maxLength={100000}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && (sendKey === "enter" || e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              if (!busy) void send(false);
            }
          }}
        />
        <div className="flex flex-wrap gap-2">
          <Button disabled={busy || !body.trim()} onClick={() => void send(false)}>
            Send to people
          </Button>
          {thread.kind !== "discussion" && (
            <Button
              variant="secondary"
              disabled={busy || !body.trim() || !selected.length}
              onClick={() => void send(true)}
            >
              Ask AI
            </Button>
          )}
        </div>
        {thread.kind !== "discussion" && (
          <fieldset className="flex flex-wrap gap-3">
            <legend className="mb-2 text-sm">Authorized tasks for AI context</legend>
            {tasks.map((t) => (
              <label key={t.id} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={selected.includes(t.id)}
                  onChange={(e) =>
                    setSelected(e.target.checked ? [...selected, t.id] : selected.filter((x) => x !== t.id))
                  }
                />
                {t.name || t.description}
              </label>
            ))}
          </fieldset>
        )}
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-sm">
            Upload attachment{" "}
            <input
              type="file"
              disabled={busy}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void upload(file);
              }}
            />
          </label>
          <select
            aria-label="Attach file to message"
            value={attachment}
            onChange={(e) => setAttachment(e.target.value)}
            className="rounded-md border bg-background p-2 text-sm"
          >
            <option value="">No attachment</option>
            {attachments.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} ({a.size} bytes)
              </option>
            ))}
          </select>
        </div>
        <p className="text-muted-foreground text-xs">
          Private AI threads are visible to their owner and system administrators. Attachments inherit thread access. AI
          receives text and selected task snapshots; attachment bytes stay outside model context.
        </p>
      </CardContent>
    </Card>
  );
}
