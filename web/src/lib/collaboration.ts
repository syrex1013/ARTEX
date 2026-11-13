import { http } from "@/lib/api";

export interface EngagementTask {
  id: number;
  name: string;
  description: string;
  goal: string;
  status: string;
  created_at: string;
}
export interface SnapshotFinding {
  id: number;
  task_id: number;
  name: string;
  severity: string;
  status: string;
  verification_status: string;
  verification_note: string;
  summary: string;
  evidence: string;
  evidence_version: number;
}
export interface Snapshot {
  tasks: EngagementTask[];
  findings: SnapshotFinding[];
  assets: { id: number; type: string; domain?: string; ip?: string; url?: string }[];
  limitations: string;
}
export interface SavedReport {
  id: number;
  family_id: string;
  version: number;
  title: string;
  author: string;
  author_id: number;
  task_ids: number[];
  status: string;
  model: string;
  usage: Record<string, number>;
  error: string;
  approved_at: string | null;
  reviewer_id: number | null;
  created_at: string;
  output?: string;
  stale?: boolean;
  snapshot?: Snapshot;
}
export interface DiscussionThread {
  id: number;
  title: string;
  kind: string;
  owner: string;
  owner_id: number;
}
export interface DiscussionMessage {
  id: number;
  author_id: number;
  author: string;
  kind: string;
  body: string;
  state: string;
  error: string;
  client_id: string;
  created_at: string;
  attachment_id: string | null;
}
export interface ThreadAttachment {
  id: string;
  name: string;
  size: number;
}
export function engagementPath(id: number) {
  return `/collab/engagements/${id}`;
}
export async function downloadCollaboration(path: string, filename: string) {
  const token = localStorage.getItem("artex_token");
  const response = await fetch(`/api${path}`, { headers: token ? { Authorization: `Bearer ${token}` } : {} });
  if (!response.ok) throw new Error(`Download failed (${response.status})`);
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export const mutation = <T = { ok: boolean }>(path: string, method: string, value?: unknown) =>
  http<T>(path, { method, ...(value === undefined ? {} : { body: JSON.stringify(value) }) });

export function collaborationTime(value: string) {
  const timezone = typeof window === "undefined" ? undefined : localStorage.getItem("artex_timezone") || undefined;
  return new Intl.DateTimeFormat(undefined, { timeZone: timezone, dateStyle: "medium", timeStyle: "short" }).format(
    new Date(value),
  );
}
