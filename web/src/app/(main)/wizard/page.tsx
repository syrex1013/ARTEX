"use client";

import * as React from "react";

import {
  ArrowRightIcon,
  BellIcon,
  CheckCircle2Icon,
  ChevronRightIcon,
  CpuIcon,
  DownloadIcon,
  RocketIcon,
  UploadIcon,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { api } from "@/lib/api";
import type { ConfigBundle, LLMProfile } from "@/lib/types";
import { cn } from "@/lib/utils";

const STEPS = [
  { id: "welcome", label: "Welcome" },
  { id: "llm", label: "LLM" },
  { id: "notify", label: "Notifications" },
  { id: "done", label: "Done" },
] as const;

type StepId = (typeof STEPS)[number]["id"];

const LLM_FORMATS = [
  { value: "anthropic", label: "Anthropic (Claude)" },
  { value: "openai", label: "OpenAI / Compatible" },
  { value: "ollama", label: "Ollama (local)" },
];

const NOTIFY_KINDS = [
  { value: "slack", label: "Slack" },
  { value: "discord", label: "Discord" },
  { value: "telegram", label: "Telegram" },
  { value: "email", label: "Email (SMTP)" },
  { value: "webhook", label: "Generic Webhook" },
];

function StepIndicator({ current }: { current: StepId }) {
  const currentIdx = STEPS.findIndex((s) => s.id === current);
  return (
    <nav className="flex items-center gap-2">
      {STEPS.map((step, i) => {
        const done = i < currentIdx;
        const active = i === currentIdx;
        return (
          <React.Fragment key={step.id}>
            {i > 0 && <ChevronRightIcon className="size-4 text-muted-foreground/40" />}
            <span
              className={cn(
                "text-sm font-medium transition-colors",
                active && "text-foreground",
                done && "text-primary",
                !active && !done && "text-muted-foreground/50",
              )}
            >
              {done ? <CheckCircle2Icon className="inline-block size-4 text-primary" /> : null}{" "}
              {step.label}
            </span>
          </React.Fragment>
        );
      })}
    </nav>
  );
}

export default function WizardPage() {
  const router = useRouter();
  const [step, setStep] = React.useState<StepId>("welcome");

  // LLM step state
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [llmFormat, setLlmFormat] = React.useState("anthropic");
  const [llmName, setLlmName] = React.useState("Claude (Anthropic)");
  const [llmApiKey, setLlmApiKey] = React.useState("");
  const [llmModel, setLlmModel] = React.useState("claude-opus-4-5");
  const [llmBase, setLlmBase] = React.useState("");
  const [savingLLM, setSavingLLM] = React.useState(false);
  const [llmSaved, setLlmSaved] = React.useState(false);

  // Notify step state
  const [notifyKind, setNotifyKind] = React.useState("slack");
  const [notifyName, setNotifyName] = React.useState("My Slack");
  const [notifyWebhook, setNotifyWebhook] = React.useState("");
  const [savingNotify, setSavingNotify] = React.useState(false);
  const [notifySaved, setNotifySaved] = React.useState(false);

  // Import state
  const [importing, setImporting] = React.useState(false);
  const fileRef = React.useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    // Check if wizard was already completed
    const done = localStorage.getItem("artex_wizard_done");
    if (done === "1") {
      router.replace("/function/tasks");
      return;
    }
    api.llmProfiles().then(setProfiles).catch(() => {});
  }, [router]);

  async function saveLLM() {
    if (!llmApiKey && llmFormat !== "ollama") {
      toast.error("API key is required");
      return;
    }
    setSavingLLM(true);
    try {
      const payload: Parameters<typeof api.saveLLMProfile>[0] = {
        name: llmName || "Default",
        format: llmFormat,
        model: llmModel,
        base_url: llmBase || undefined,
        api_key: llmApiKey || undefined,
        max_tokens: 8192,
      };
      const saved = await api.saveLLMProfile(payload);
      await api.activateLLMProfile(String(saved.id));
      setLlmSaved(true);
      toast.success("LLM profile saved and activated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save LLM profile");
    } finally {
      setSavingLLM(false);
    }
  }

  async function saveNotify() {
    if (!notifyWebhook) {
      toast.error("Webhook URL is required");
      return;
    }
    setSavingNotify(true);
    try {
      await api.notifyCreateChannel({
        name: notifyName || notifyKind,
        kind: notifyKind,
        enabled: true,
        config: { webhook_url: notifyWebhook },
      });
      setNotifySaved(true);
      toast.success("Notification channel saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save channel");
    } finally {
      setSavingNotify(false);
    }
  }

  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setImporting(true);
    try {
      const text = await file.text();
      const bundle: ConfigBundle = JSON.parse(text);
      await api.importConfig(bundle, false);
      toast.success("Settings imported successfully");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  function finish() {
    localStorage.setItem("artex_wizard_done", "1");
    router.replace("/function/tasks");
  }

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center bg-background p-6">
      <div className="w-full max-w-2xl space-y-8">
        {/* Header */}
        <div className="space-y-2 text-center">
          <div className="flex items-center justify-center gap-2">
            <RocketIcon className="size-6 text-primary" />
            <h1 className="text-2xl font-semibold tracking-tight">ARTEX Setup</h1>
          </div>
          <p className="text-sm text-muted-foreground">Configure ARTEX to get started. You can change all settings later.</p>
        </div>

        <div className="flex justify-center">
          <StepIndicator current={step} />
        </div>

        {/* Step: Welcome */}
        {step === "welcome" && (
          <Card>
            <CardHeader>
              <CardTitle>Welcome to ARTEX</CardTitle>
              <CardDescription>Your AI-powered penetration testing platform. This short wizard helps you configure the essentials in a few steps.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-3">
                {[
                  { icon: CpuIcon, title: "LLM Configuration", desc: "Connect your AI model (Claude, OpenAI, or local Ollama)" },
                  { icon: BellIcon, title: "Notifications", desc: "Set up Slack, Discord, or other channels for alerts" },
                ].map(({ icon: Icon, title, desc }) => (
                  <div key={title} className="flex items-start gap-3 rounded-lg border p-3">
                    <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
                    <div>
                      <p className="text-sm font-medium">{title}</p>
                      <p className="text-xs text-muted-foreground">{desc}</p>
                    </div>
                  </div>
                ))}
              </div>

              <Separator />

              <div className="space-y-2">
                <p className="text-sm font-medium">Import existing settings</p>
                <p className="text-xs text-muted-foreground">If you have a previous ARTEX config export, import it now to restore your LLM profiles, API keys, and preferences.</p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => fileRef.current?.click()} disabled={importing}>
                    <UploadIcon className="mr-1.5 size-4" />
                    {importing ? "Importing..." : "Import config backup"}
                  </Button>
                  <input ref={fileRef} type="file" accept=".json,application/json" className="hidden" onChange={handleImport} />
                </div>
              </div>

              <div className="flex justify-end">
                <Button onClick={() => setStep("llm")}>
                  Next: LLM setup <ArrowRightIcon className="ml-1.5 size-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Step: LLM */}
        {step === "llm" && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <CpuIcon className="size-5" /> LLM Configuration
              </CardTitle>
              <CardDescription>
                Configure an AI model to power ARTEX agents.{" "}
                {profiles.length > 0 && (
                  <Badge variant="secondary" className="ml-1">
                    {profiles.length} profile{profiles.length > 1 ? "s" : ""} already configured
                  </Badge>
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {profiles.length > 0 && (
                <div className="rounded-lg border border-primary/30 bg-primary/5 p-3 text-sm text-primary">
                  You already have {profiles.length} LLM profile(s). You can skip this step or add another.
                </div>
              )}

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label>Provider / Format</Label>
                  <Select value={llmFormat} onValueChange={setLlmFormat}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {LLM_FORMATS.map((f) => (
                        <SelectItem key={f.value} value={f.value}>
                          {f.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label>Profile name</Label>
                  <Input value={llmName} onChange={(e) => setLlmName(e.target.value)} placeholder="My Claude profile" />
                </div>
              </div>

              <div className="space-y-1.5">
                <Label>Model</Label>
                <Input
                  value={llmModel}
                  onChange={(e) => setLlmModel(e.target.value)}
                  placeholder={llmFormat === "anthropic" ? "claude-opus-4-5" : llmFormat === "ollama" ? "llama3.1:8b" : "gpt-4o"}
                />
              </div>

              {llmFormat !== "ollama" && (
                <div className="space-y-1.5">
                  <Label>API Key</Label>
                  <Input
                    type="password"
                    value={llmApiKey}
                    onChange={(e) => setLlmApiKey(e.target.value)}
                    placeholder={llmFormat === "anthropic" ? "sk-ant-..." : "sk-..."}
                    autoComplete="off"
                  />
                </div>
              )}

              {llmFormat !== "anthropic" && (
                <div className="space-y-1.5">
                  <Label>Base URL {llmFormat === "ollama" ? "" : "(optional)"}</Label>
                  <Input
                    value={llmBase}
                    onChange={(e) => setLlmBase(e.target.value)}
                    placeholder={llmFormat === "ollama" ? "http://localhost:11434" : "https://api.openai.com/v1"}
                  />
                </div>
              )}

              <div className="flex justify-between pt-2">
                <Button variant="outline" onClick={() => setStep("welcome")}>
                  Back
                </Button>
                <div className="flex gap-2">
                  {!llmSaved && (
                    <Button variant="outline" onClick={() => setStep("notify")}>
                      Skip
                    </Button>
                  )}
                  <Button onClick={llmSaved ? () => setStep("notify") : saveLLM} disabled={savingLLM}>
                    {llmSaved ? (
                      <>
                        <CheckCircle2Icon className="mr-1.5 size-4 text-green-500" /> Next: Notifications
                      </>
                    ) : savingLLM ? (
                      "Saving..."
                    ) : (
                      <>
                        Save &amp; continue <ArrowRightIcon className="ml-1.5 size-4" />
                      </>
                    )}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Step: Notifications */}
        {step === "notify" && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <BellIcon className="size-5" /> Notifications
              </CardTitle>
              <CardDescription>Get alerts for findings, budget limits, and testing window changes. You can skip this and configure it later in System → Notifications.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label>Channel type</Label>
                  <Select value={notifyKind} onValueChange={setNotifyKind}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {NOTIFY_KINDS.map((k) => (
                        <SelectItem key={k.value} value={k.value}>
                          {k.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label>Channel name</Label>
                  <Input value={notifyName} onChange={(e) => setNotifyName(e.target.value)} placeholder="My Slack" />
                </div>
              </div>

              <div className="space-y-1.5">
                <Label>Webhook URL</Label>
                <Input
                  value={notifyWebhook}
                  onChange={(e) => setNotifyWebhook(e.target.value)}
                  placeholder="https://hooks.slack.com/services/..."
                  autoComplete="off"
                />
              </div>

              <div className="flex justify-between pt-2">
                <Button variant="outline" onClick={() => setStep("llm")}>
                  Back
                </Button>
                <div className="flex gap-2">
                  <Button variant="outline" onClick={() => setStep("done")}>
                    Skip
                  </Button>
                  <Button onClick={notifySaved ? () => setStep("done") : saveNotify} disabled={savingNotify}>
                    {notifySaved ? (
                      <>
                        <CheckCircle2Icon className="mr-1.5 size-4 text-green-500" /> Next: Finish
                      </>
                    ) : savingNotify ? (
                      "Saving..."
                    ) : (
                      <>
                        Save &amp; continue <ArrowRightIcon className="ml-1.5 size-4" />
                      </>
                    )}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Step: Done */}
        {step === "done" && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <CheckCircle2Icon className="size-5 text-green-500" /> You&apos;re all set!
              </CardTitle>
              <CardDescription>ARTEX is configured and ready. You can always revisit these settings under System → LLM and System → Notifications.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-3">
                {[
                  { href: "/function/tasks", label: "Start a task", desc: "Kick off your first AI-assisted pentest task" },
                  { href: "/function/engagements", label: "Create an engagement", desc: "Manage scope, windows, and budget for a full engagement" },
                  { href: "/function/workspace", label: "Open workspace", desc: "Notes, files, and engagement artifacts" },
                  { href: "/system/settings", label: "Settings", desc: "Fine-tune ARTEX behaviour, proxies, and Python interpreter" },
                ].map(({ href, label, desc }) => (
                  <button
                    key={href}
                    onClick={() => { localStorage.setItem("artex_wizard_done", "1"); router.push(href); }}
                    className="flex items-center gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent"
                  >
                    <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                    <div>
                      <p className="text-sm font-medium">{label}</p>
                      <p className="text-xs text-muted-foreground">{desc}</p>
                    </div>
                  </button>
                ))}
              </div>

              <div className="flex justify-center">
                <Button size="lg" onClick={finish}>
                  <RocketIcon className="mr-2 size-4" />
                  Launch ARTEX
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
