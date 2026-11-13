// Centralised status → color/label semantics, reused across the whole app.
// Spec §8.3: intent / coverage / task / severity each have a consistent color set.

export type Tone = "neutral" | "blue" | "green" | "amber" | "red" | "rose" | "violet" | "slate";

// Tone classes reference semantic tokens (--tone-*) declared in globals.css so
// dark mode and theme presets recolor every status surface from one place.
export const toneClasses: Record<Tone, string> = {
  neutral: "bg-muted text-muted-foreground border-transparent",
  blue: "bg-tone-info/15 text-tone-info border-tone-info/20",
  green: "bg-tone-success/15 text-tone-success border-tone-success/20",
  amber: "bg-tone-warn/15 text-tone-warn border-tone-warn/20",
  red: "bg-tone-danger/15 text-tone-danger border-tone-danger/20",
  // rose is used for "Critical" — a solid high-emphasis fill clearly above the soft red outline of "High".
  rose: "bg-tone-critical text-white border-tone-critical",
  violet: "bg-tone-accent/15 text-tone-accent border-tone-accent/20",
  slate: "bg-tone-muted/15 text-tone-muted border-tone-muted/20",
};

export const toneDot: Record<Tone, string> = {
  neutral: "bg-muted-foreground",
  blue: "bg-tone-info",
  green: "bg-tone-success",
  amber: "bg-tone-warn",
  red: "bg-tone-danger",
  // rose badge is a solid critical fill; a white dot keeps contrast on it.
  rose: "bg-white",
  violet: "bg-tone-accent",
  slate: "bg-tone-muted",
};

interface StatusMeta {
  label: string;
  tone: Tone;
}

const intent: Record<string, StatusMeta> = {
  open: { label: "Open", tone: "slate" },
  running: { label: "Running", tone: "blue" },
  paused: { label: "Paused", tone: "amber" },
  done: { label: "Completed", tone: "green" },
  // blocked = model/API/network failures exhausted the retries; this intent barely explored anything (not an interception).
  blocked: { label: "Error", tone: "red" },
  // exhausted = the step/time budget cut the run short midway, only partial results written back (not "direction exhausted").
  exhausted: { label: "Budget exhausted", tone: "violet" },
  // stopped = legacy soft-deleted state (kept for historical data).
  stopped: { label: "Stopped", tone: "slate" },
  // deleted = the user soft-deleted this intent (nodes and lineage kept; see delete_reason for the reason).
  deleted: { label: "Deleted", tone: "slate" },
};

const task: Record<string, StatusMeta> = {
  created: { label: "Created", tone: "slate" },
  queued: { label: "Queued", tone: "amber" },
  running: { label: "Running", tone: "blue" },
  paused: { label: "Paused", tone: "amber" },
  done: { label: "Completed", tone: "green" },
  failed: { label: "Failed", tone: "red" },
  timeout: { label: "Timed out", tone: "amber" },
};

const severity: Record<string, StatusMeta> = {
  critical: { label: "Critical", tone: "rose" },
  high: { label: "High", tone: "red" },
  medium: { label: "Medium", tone: "amber" },
  low: { label: "Low", tone: "slate" },
};

const finding: Record<string, StatusMeta> = {
  pending: { label: "Pending", tone: "amber" },
  in_progress: { label: "In progress", tone: "blue" },
  confirmed: { label: "Confirmed", tone: "red" },
  resolved: { label: "Resolved", tone: "green" },
  fixed: { label: "Fixed", tone: "green" },
  false_positive: { label: "False positive", tone: "slate" },
  ignored: { label: "Ignored", tone: "neutral" },
  duplicate: { label: "Duplicate", tone: "neutral" },
  risk_accepted: { label: "Risk accepted", tone: "violet" },
};

const engine: Record<string, StatusMeta> = {
  exploring: { label: "Exploring", tone: "blue" },
  paused: { label: "Paused", tone: "amber" },
  stalled: { label: "Stalled", tone: "red" },
  idle: { label: "Idle", tone: "neutral" },
};

const goal: Record<string, StatusMeta> = {
  open: { label: "In progress", tone: "blue" },
  met: { label: "Met", tone: "green" },
  abandoned: { label: "Abandoned", tone: "slate" },
};

const audit: Record<string, StatusMeta> = {
  allow: { label: "Allow", tone: "green" },
  block: { label: "Block", tone: "red" },
};

const node: Record<string, StatusMeta> = {
  observed: { label: "Observed", tone: "slate" },
  confirmed: { label: "Confirmed", tone: "green" },
  tombstoned: { label: "Tombstoned", tone: "neutral" },
};

// Notification delivery states. sending uses blue rather than amber: it is not "a problem"
// but "claimed and sending now", which must stay distinguishable from pending's waiting.
const delivery: Record<string, StatusMeta> = {
  pending: { label: "Pending", tone: "amber" },
  sending: { label: "Sending", tone: "blue" },
  sent: { label: "Sent", tone: "green" },
  failed: { label: "Failed", tone: "red" },
  skipped: { label: "Skipped", tone: "neutral" },
};

const maps = {
  intent,
  task,
  severity,
  finding,
  engine,
  goal,
  audit,
  node,
  delivery,
} as const;

export type StatusDomain = keyof typeof maps;

export function statusMeta(domain: StatusDomain, key: string): StatusMeta {
  return maps[domain][key] ?? { label: key, tone: "neutral" };
}
