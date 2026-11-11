"use client";

import * as React from "react";

import {
  AlertTriangleIcon,
  DownloadIcon,
  RefreshCwIcon,
  SearchIcon,
  SettingsIcon,
  SparklesIcon,
  Trash2Icon,
} from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { api } from "@/lib/api";
import type { MarketPreview, MarketSkill, MarketSource } from "@/lib/types";
import { cn } from "@/lib/utils";

type Confirm = { kind: "install" | "update" | "uninstall"; skill: MarketSkill } | null;

const keyOf = (s: MarketSkill) => `${s.source}:${s.path}`;

function fmtBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function MarketplaceSheet({
  open,
  onOpenChange,
  onChanged,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called after an install, update or uninstall so the skills tree can reload. */
  onChanged: () => void;
}) {
  const [sources, setSources] = React.useState<MarketSource[]>([]);
  const [source, setSource] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [debounced, setDebounced] = React.useState("");
  const [skills, setSkills] = React.useState<MarketSkill[]>([]);
  const [errors, setErrors] = React.useState<{ source: string; error: string }[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [selected, setSelected] = React.useState<MarketSkill | null>(null);
  const [preview, setPreview] = React.useState<MarketPreview | null>(null);
  const [previewLoading, setPreviewLoading] = React.useState(false);
  const [confirm, setConfirm] = React.useState<Confirm>(null);
  const [busy, setBusy] = React.useState<string | null>(null);

  React.useEffect(() => {
    const t = setTimeout(() => setDebounced(query.trim()), 250);
    return () => clearTimeout(t);
  }, [query]);

  const loadSources = React.useCallback(() => {
    api
      .marketSources()
      .then(setSources)
      .catch(() => undefined);
  }, []);

  const search = React.useCallback(
    (refresh = false) => {
      setLoading(true);
      api
        .marketSearch(debounced, source || undefined, refresh)
        .then((r) => {
          setSkills(r.skills ?? []);
          setErrors(r.errors ?? []);
        })
        .catch((e: Error) => {
          setSkills([]);
          setErrors([{ source: "", error: e.message }]);
        })
        .finally(() => setLoading(false));
    },
    [debounced, source],
  );

  React.useEffect(() => {
    if (open) loadSources();
  }, [open, loadSources]);

  React.useEffect(() => {
    if (open) search(false);
  }, [open, search]);

  // Keep the detail pane's list entry in sync after installs.
  const current = selected ? (skills.find((s) => keyOf(s) === keyOf(selected)) ?? selected) : null;

  const select = (s: MarketSkill) => {
    setSelected(s);
    setPreview(null);
    setPreviewLoading(true);
    api
      .marketPreview(s.source, s.path)
      .then(setPreview)
      .catch((e: Error) => toast.error(`Preview failed: ${e.message}`))
      .finally(() => setPreviewLoading(false));
  };

  const run = async () => {
    if (!confirm) return;
    const { kind, skill } = confirm;
    setConfirm(null);
    setBusy(keyOf(skill));
    try {
      if (kind === "uninstall") {
        await api.deleteSkill(skill.name);
        toast.success(`Uninstalled ${skill.name}`);
      } else {
        const r = await api.marketInstall(skill.source, skill.path, kind === "update");
        toast.success(
          `${kind === "update" ? "Updated" : "Installed"} ${r.name}. It is hidden from agents until you enable it for an agent.`,
        );
      }
      onChanged();
      search(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex flex-col gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-5xl">
        <SheetHeader className="border-b p-4 pr-12">
          <SheetTitle className="flex items-center gap-2">
            <SparklesIcon className="size-4" />
            Skill marketplace
          </SheetTitle>
          <SheetDescription>
            Public skill repositories on GitHub. Nothing is installed without your confirmation, and installed skills
            stay hidden from agents until you enable them.
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-wrap items-center gap-2 border-b p-3">
          <div className="relative min-w-48 flex-1">
            <SearchIcon className="absolute top-2.5 left-2.5 size-3.5 text-muted-foreground" />
            <Input
              className="h-8 pl-8 text-sm"
              placeholder="Search skills"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <select
            className="h-8 rounded-md border bg-background px-2 text-sm"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            aria-label="Source"
          >
            <option value="">All sources</option>
            {sources.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label || s.repo}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            variant="outline"
            disabled={loading}
            onClick={() => search(true)}
            title="Re-read the repositories"
          >
            <RefreshCwIcon className={cn("size-3.5", loading && "animate-spin")} />
            Refresh
          </Button>
          <SourcesPopover
            sources={sources}
            onChanged={() => {
              loadSources();
              search(true);
            }}
          />
        </div>

        {errors.length > 0 && (
          <div className="space-y-1 border-b bg-destructive/10 p-3 text-xs">
            {errors.map((e) => (
              <p key={`${e.source}-${e.error}`} className="flex gap-2 text-destructive">
                <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
                <span>
                  {e.source ? <b>{e.source}: </b> : null}
                  {e.error}
                </span>
              </p>
            ))}
          </div>
        )}

        <div className="flex min-h-0 flex-1">
          <ScrollArea className="w-full border-r md:w-1/2">
            <div className="space-y-2 p-3">
              {loading && skills.length === 0 && (
                <p className="p-4 text-center text-muted-foreground text-sm">Loading…</p>
              )}
              {!loading && skills.length === 0 && errors.length === 0 && (
                <p className="p-4 text-center text-muted-foreground text-sm">
                  {sources.length === 0
                    ? "No sources configured. Add a GitHub repository under Sources."
                    : "No skills match."}
                </p>
              )}
              {skills.map((s) => (
                <div
                  key={keyOf(s)}
                  className={cn(
                    "w-full cursor-pointer rounded-lg border p-3 text-left transition-colors hover:bg-accent/50",
                    current && keyOf(current) === keyOf(s) && "border-primary bg-accent/50",
                  )}
                  onClick={() => select(s)}
                  onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && select(s)}
                  role="button"
                  tabIndex={0}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span className="font-medium text-sm">{s.name}</span>
                        {s.installed && <Badge variant="secondary">Installed</Badge>}
                        {s.update_available && <Badge>Update</Badge>}
                        {s.has_scripts && <Badge variant="outline">scripts</Badge>}
                        {s.requested > 0 && <Badge variant="outline">asked for {s.requested}×</Badge>}
                      </div>
                      <p className="mt-1 line-clamp-2 text-muted-foreground text-xs">{s.description}</p>
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {s.repo} · {s.files} files · {fmtBytes(s.bytes)}
                      </p>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </ScrollArea>

          <div className="hidden min-w-0 flex-1 flex-col md:flex">
            {!current ? (
              <p className="m-auto p-6 text-center text-muted-foreground text-sm">
                Select a skill to read it before installing.
              </p>
            ) : (
              <>
                <div className="flex flex-wrap items-center justify-between gap-2 border-b p-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium text-sm">{current.name}</p>
                    <p className="truncate text-muted-foreground text-xs">
                      {current.repo}/{current.path || "."} @ {current.commit.slice(0, 7)}
                      {current.license ? ` · ${current.license}` : ""}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    {current.installed && (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={busy !== null}
                        onClick={() => setConfirm({ kind: "uninstall", skill: current })}
                      >
                        <Trash2Icon className="size-3.5" />
                        Uninstall
                      </Button>
                    )}
                    {current.installed && current.update_available && (
                      <Button
                        size="sm"
                        disabled={busy !== null}
                        onClick={() => setConfirm({ kind: "update", skill: current })}
                      >
                        <RefreshCwIcon className="size-3.5" />
                        Update
                      </Button>
                    )}
                    {!current.installed && (
                      <Button
                        size="sm"
                        disabled={busy !== null}
                        onClick={() => setConfirm({ kind: "install", skill: current })}
                      >
                        <DownloadIcon className="size-3.5" />
                        {busy === keyOf(current) ? "Installing…" : "Install"}
                      </Button>
                    )}
                  </div>
                </div>
                <ScrollArea className="flex-1">
                  <div className="space-y-4 p-3">
                    {previewLoading && <p className="text-muted-foreground text-sm">Loading preview…</p>}
                    {preview && (
                      <>
                        <div>
                          <p className="mb-1 font-medium text-xs">Files ({preview.files.length})</p>
                          <ul className="space-y-0.5 font-mono text-[11px]">
                            {preview.files.map((f) => (
                              <li
                                key={f.path}
                                className={cn(
                                  "flex justify-between",
                                  f.path.startsWith("scripts/") && "text-amber-600 dark:text-amber-400",
                                )}
                              >
                                <span className="truncate">{f.path}</span>
                                <span className="ml-2 shrink-0 text-muted-foreground">{fmtBytes(f.size)}</span>
                              </li>
                            ))}
                          </ul>
                        </div>
                        <div>
                          <p className="mb-1 font-medium text-xs">SKILL.md</p>
                          <pre className="whitespace-pre-wrap break-words rounded-md border bg-muted/40 p-3 text-xs">
                            {preview.skill_md}
                          </pre>
                        </div>
                      </>
                    )}
                  </div>
                </ScrollArea>
              </>
            )}
          </div>
        </div>

        <AlertDialog open={!!confirm} onOpenChange={(o) => !o && setConfirm(null)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {confirm?.kind === "uninstall"
                  ? `Uninstall ${confirm.skill.name}?`
                  : `${confirm?.kind === "update" ? "Update" : "Install"} ${confirm?.skill.name}?`}
              </AlertDialogTitle>
              <AlertDialogDescription asChild>
                <div className="space-y-2 text-sm">
                  {confirm?.kind === "uninstall" ? (
                    <p>
                      This deletes the skill&apos;s folder, including any local edits, and removes it from every agent.
                    </p>
                  ) : (
                    <>
                      <p>
                        From <b>{confirm?.skill.repo}</b> at commit {confirm?.skill.commit.slice(0, 7)} (
                        {confirm?.skill.files} files).
                      </p>
                      {confirm?.skill.has_scripts && (
                        <p className="flex gap-2 text-amber-600 dark:text-amber-400">
                          <AlertTriangleIcon className="mt-0.5 size-4 shrink-0" />
                          This skill includes scripts that agents can run. Read them in the preview first.
                        </p>
                      )}
                      <p>The skill stays hidden from agents until you enable it for an agent on the Skill page.</p>
                      {confirm?.kind === "update" && <p>Local edits to this skill will be overwritten.</p>}
                    </>
                  )}
                </div>
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={() => void run()}>
                {confirm?.kind === "uninstall" ? "Uninstall" : confirm?.kind === "update" ? "Update" : "Install"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </SheetContent>
    </Sheet>
  );
}

function SourcesPopover({ sources, onChanged }: { sources: MarketSource[]; onChanged: () => void }) {
  const [repo, setRepo] = React.useState("");
  const [ref, setRef] = React.useState("");
  const [adding, setAdding] = React.useState(false);

  const add = async () => {
    if (!repo.trim()) return;
    setAdding(true);
    try {
      const r = await api.marketAddSource(repo.trim(), ref.trim());
      toast.success(`Added ${r.source.repo} (${r.skills} skills)`);
      setRepo("");
      setRef("");
      onChanged();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setAdding(false);
    }
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm" variant="outline">
          <SettingsIcon className="size-3.5" />
          Sources
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-3">
        <div>
          <p className="font-medium text-sm">Skill sources</p>
          <p className="text-muted-foreground text-xs">
            Public GitHub repositories containing folders with a SKILL.md.
          </p>
        </div>
        <ul className="space-y-1">
          {sources.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-2 text-xs">
              <span className="truncate">
                {s.repo}
                {s.ref ? `@${s.ref}` : ""}
              </span>
              <Button
                size="icon"
                variant="ghost"
                className="size-6"
                aria-label={`Remove ${s.repo}`}
                onClick={() =>
                  api
                    .marketRemoveSource(s.id)
                    .then(onChanged)
                    .catch((e: Error) => toast.error(e.message))
                }
              >
                <Trash2Icon className="size-3" />
              </Button>
            </li>
          ))}
          {sources.length === 0 && <li className="text-muted-foreground text-xs">No sources.</li>}
        </ul>
        <div className="space-y-2 border-t pt-3">
          <Input
            className="h-8 text-xs"
            placeholder="owner/repo"
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
          />
          <Input
            className="h-8 text-xs"
            placeholder="branch or tag (optional)"
            value={ref}
            onChange={(e) => setRef(e.target.value)}
          />
          <div className="flex justify-between gap-2">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setRepo("anthropics/skills");
                setRef("");
              }}
            >
              Use anthropics/skills
            </Button>
            <Button size="sm" disabled={adding || !repo.trim()} onClick={() => void add()}>
              {adding ? "Checking…" : "Add"}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
