"use client";

import * as React from "react";

import { AlertTriangleIcon, DownloadIcon, UploadIcon } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { applyImportedPreferences, collectPreferences } from "@/lib/preferences/preferences-transfer";
import type { ConfigBundle, ConfigImportResult, ConfigSection } from "@/lib/types";
import { usePreferencesStore } from "@/stores/preferences/preferences-provider";

const SECTION_LABELS: { key: ConfigSection; label: string; hint: string }[] = [
  { key: "settings", label: "System settings", hint: "Toggles, proxies, workers, web-search keys, push options" },
  { key: "llm", label: "LLM profiles", hint: "Profiles including API keys, plus the retry policy" },
  { key: "mcp", label: "MCP servers", hint: "Commands, URLs and environment variables / headers" },
  { key: "notify", label: "Notification channels", hint: "Webhook URLs, bot tokens, SMTP credentials" },
];

const SECTION_NAMES: Record<ConfigSection, string> = {
  settings: "Settings",
  llm: "LLM",
  mcp: "MCP",
  notify: "Notifications",
};

function isBundle(v: unknown): v is ConfigBundle {
  return !!v && typeof v === "object" && (v as ConfigBundle).format === "artex-config";
}

export function ConfigBackupCard({ onImported }: { onImported?: () => void }) {
  const prefsState = usePreferencesStore((s) => s);
  const [sections, setSections] = React.useState<Record<ConfigSection, boolean>>({
    settings: true,
    llm: true,
    mcp: true,
    notify: true,
  });
  const [includePrefs, setIncludePrefs] = React.useState(true);
  const [confirmExport, setConfirmExport] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [pending, setPending] = React.useState<{
    bundle: ConfigBundle;
    preview: ConfigImportResult;
    file: string;
  } | null>(null);
  const fileRef = React.useRef<HTMLInputElement>(null);

  const selected = (Object.keys(sections) as ConfigSection[]).filter((k) => sections[k]);

  const doExport = async () => {
    setConfirmExport(false);
    setBusy(true);
    try {
      const bundle: ConfigBundle =
        selected.length > 0
          ? await api.exportConfig(selected)
          : { format: "artex-config", version: 1, includes_secrets: false, exported_at: new Date().toISOString() };
      if (includePrefs) bundle.preferences = collectPreferences(prefsState);
      const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `artex-config-${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, "")}.json`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      toast.success("Configuration exported. The file contains plaintext secrets, so store it securely.");
    } catch (e) {
      toast.error(`Export failed: ${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  };

  const onFile = async (file: File | undefined) => {
    if (!file) return;
    setBusy(true);
    try {
      let parsed: unknown;
      try {
        parsed = JSON.parse(await file.text());
      } catch {
        throw new Error("The file is not valid JSON");
      }
      if (!isBundle(parsed)) throw new Error("Not an ARTEX configuration file");
      const preview = await api.importConfig(parsed, true);
      setPending({ bundle: parsed, preview, file: file.name });
    } catch (e) {
      toast.error(`Import failed: ${(e as Error).message}`);
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };

  const doImport = async () => {
    if (!pending) return;
    const { bundle } = pending;
    setBusy(true);
    try {
      const res = await api.importConfig(bundle, false);
      const applied = applyImportedPreferences(bundle.preferences, prefsState);
      const errors = Object.values(res.sections).flatMap((s) => s?.errors ?? []);
      const touched = Object.values(res.sections).reduce((n, s) => n + (s?.created ?? 0) + (s?.updated ?? 0), 0);
      if (errors.length > 0) {
        toast.warning(`Imported ${touched} item(s) with ${errors.length} problem(s). See the details below.`);
        setPending({ bundle, preview: res, file: pending.file });
      } else {
        toast.success(`Imported ${touched} item(s)${applied.length ? ` and ${applied.length} preference(s)` : ""}.`);
        setPending(null);
      }
      onImported?.();
    } catch (e) {
      toast.error(`Import failed: ${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  };

  const done = pending && !pending.preview.dry_run;
  const prefCount = pending?.bundle.preferences ? Object.keys(pending.bundle.preferences).length : 0;

  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <DownloadIcon className="size-4" />
          Configuration backup
        </CardTitle>
        <CardDescription>
          Export this instance&apos;s configuration to a JSON file, or restore one. Tasks, findings and traffic are not
          included; use the backup script for those.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          {SECTION_LABELS.map((s) => (
            <div key={s.key} className="flex items-start gap-2">
              <Checkbox
                id={`cfg-${s.key}`}
                checked={sections[s.key]}
                onCheckedChange={(v) => setSections((p) => ({ ...p, [s.key]: v === true }))}
              />
              <Label htmlFor={`cfg-${s.key}`} className="flex flex-col items-start gap-0.5 font-normal">
                <span className="font-medium text-sm">{s.label}</span>
                <span className="text-muted-foreground text-xs">{s.hint}</span>
              </Label>
            </div>
          ))}
          <div className="flex items-start gap-2">
            <Checkbox id="cfg-prefs" checked={includePrefs} onCheckedChange={(v) => setIncludePrefs(v === true)} />
            <Label htmlFor="cfg-prefs" className="flex flex-col items-start gap-0.5 font-normal">
              <span className="font-medium text-sm">Appearance &amp; browser preferences</span>
              <span className="text-muted-foreground text-xs">
                Theme, font, layout and chat key binding of this browser
              </span>
            </Label>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            disabled={busy || (selected.length === 0 && !includePrefs)}
            onClick={() => (selected.length > 0 ? setConfirmExport(true) : void doExport())}
          >
            <DownloadIcon className="size-4" />
            Export
          </Button>
          <Button size="sm" variant="outline" disabled={busy} onClick={() => fileRef.current?.click()}>
            <UploadIcon className="size-4" />
            Import…
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            onChange={(e) => void onFile(e.target.files?.[0])}
          />
        </div>
      </CardContent>

      <AlertDialog open={confirmExport} onOpenChange={setConfirmExport}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2">
              <AlertTriangleIcon className="size-5 text-amber-500" />
              This file contains plaintext secrets
            </AlertDialogTitle>
            <AlertDialogDescription>
              The export includes API keys, tokens, webhook URLs and passwords exactly as they are stored. Anyone who
              gets the file can use them. Keep it out of shared folders and version control, and delete it when you are
              done.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => void doExport()}>Export with secrets</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={!!pending} onOpenChange={(o) => !o && setPending(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{done ? "Import finished" : "Import configuration?"}</AlertDialogTitle>
            <AlertDialogDescription>
              {done
                ? `Result for ${pending?.file}.`
                : `${pending?.file}: existing items with the same name are overwritten, nothing is deleted.`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {pending && (
            <div className="space-y-2 text-sm">
              {(Object.keys(pending.preview.sections) as ConfigSection[]).map((k) => {
                const s = pending.preview.sections[k];
                if (!s) return null;
                return (
                  <div key={k}>
                    <div className="flex justify-between">
                      <span className="font-medium">{SECTION_NAMES[k]}</span>
                      <span className="text-muted-foreground">
                        {s.created} new · {s.updated} updated · {s.skipped} skipped
                      </span>
                    </div>
                    {s.errors?.map((e) => (
                      <p key={e} className="text-destructive text-xs">
                        {e}
                      </p>
                    ))}
                  </div>
                );
              })}
              {prefCount > 0 && (
                <div className="flex justify-between">
                  <span className="font-medium">Appearance</span>
                  <span className="text-muted-foreground">{prefCount} preference(s)</span>
                </div>
              )}
              {Object.keys(pending.preview.sections).length === 0 && prefCount === 0 && (
                <p className="text-muted-foreground">The file contains nothing to import.</p>
              )}
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel>{done ? "Close" : "Cancel"}</AlertDialogCancel>
            {!done && (
              <AlertDialogAction
                disabled={busy}
                onClick={(e) => {
                  e.preventDefault();
                  void doImport();
                }}
              >
                Import
              </AlertDialogAction>
            )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
