"use client";

import * as React from "react";

import {
  ClipboardCopyIcon,
  PlusIcon,
  ServerIcon,
  Trash2Icon,
  ZapIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { ImplantHints, SliverConfig } from "@/lib/types";
import { cn } from "@/lib/utils";

// ---- Add server dialog ----
function AddServerDialog({ onCreated }: { onCreated: (c: SliverConfig) => void }) {
  const [open, setOpen] = React.useState(false);
  const [name, setName] = React.useState("");
  const [host, setHost] = React.useState("");
  const [port, setPort] = React.useState("31337");
  const [operator, setOperator] = React.useState("");
  const [token, setToken] = React.useState("");
  const [caCert, setCaCert] = React.useState("");
  const [clientCert, setClientCert] = React.useState("");
  const [clientKey, setClientKey] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  async function submit() {
    if (!name || !host) { toast.error("Name and host are required"); return; }
    setSaving(true);
    try {
      const c = await api.createSliverConfig({
        name, host, port: Number(port) || 31337, operator, token,
        ca_cert: caCert, client_cert: clientCert,
      });
      onCreated(c);
      setOpen(false);
      setName(""); setHost(""); setPort("31337"); setOperator(""); setToken("");
      setCaCert(""); setClientCert(""); setClientKey("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add server");
    } finally { setSaving(false); }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm"><PlusIcon className="mr-1.5 size-4" />Add Server</Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add Sliver C2 Server</DialogTitle>
          <DialogDescription>Store connection details for a Sliver C2 server. Credentials are stored server-side and never sent back to the browser.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label>Name</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="lab-c2" />
            </div>
            <div className="grid gap-1.5">
              <Label>Operator / username</Label>
              <Input value={operator} onChange={(e) => setOperator(e.target.value)} placeholder="operator1" />
            </div>
          </div>
          <div className="grid grid-cols-[1fr_100px] gap-3">
            <div className="grid gap-1.5">
              <Label>Host / IP</Label>
              <Input value={host} onChange={(e) => setHost(e.target.value)} placeholder="10.10.10.5" />
            </div>
            <div className="grid gap-1.5">
              <Label>Port</Label>
              <Input value={port} onChange={(e) => setPort(e.target.value)} placeholder="31337" />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label>Bearer token (optional)</Label>
            <Input type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="operator token" autoComplete="off" />
          </div>
          <div className="grid gap-1.5">
            <Label>CA certificate (PEM, optional)</Label>
            <Textarea value={caCert} onChange={(e) => setCaCert(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" rows={3} className="font-mono text-xs" />
          </div>
          <div className="grid gap-1.5">
            <Label>Client certificate (PEM, optional)</Label>
            <Textarea value={clientCert} onChange={(e) => setClientCert(e.target.value)} placeholder="-----BEGIN CERTIFICATE-----" rows={2} className="font-mono text-xs" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={submit} disabled={saving}>{saving ? "Saving..." : "Save"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---- Implant Hints ----
function ImplantHintsPanel() {
  const [target, setTarget] = React.useState("");
  const [os, setOs] = React.useState("linux");
  const [hints, setHints] = React.useState<ImplantHints | null>(null);
  const [loading, setLoading] = React.useState(false);

  async function fetch() {
    setLoading(true);
    try {
      const h = await api.sliverImplantHints({ target, os });
      setHints(h);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to fetch hints");
    } finally { setLoading(false); }
  }

  function copy(text: string) {
    navigator.clipboard.writeText(text).then(() => toast.success("Copied"));
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Generate advisory implant acquisition steps for a specific target. ARTEX uses ATT&CK data and finding context to suggest Sliver commands. All execution is done by you in the Sliver console — ARTEX does not connect to the C2 server directly.
      </p>
      <div className="grid gap-3 sm:grid-cols-[1fr_140px_auto]">
        <div className="space-y-1">
          <Label>Target IP / hostname</Label>
          <Input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="10.10.10.100" />
        </div>
        <div className="space-y-1">
          <Label>OS</Label>
          <Select value={os} onValueChange={setOs}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              {["linux","windows","macos"].map((o) => (
                <SelectItem key={o} value={o}>{o}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-end">
          <Button onClick={fetch} disabled={loading}>{loading ? "Generating..." : "Generate"}</Button>
        </div>
      </div>

      {hints && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <ZapIcon className="size-4" /> {hints.target || "Target"} — {hints.os}
            </CardTitle>
            <CardDescription>{hints.strategy}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              {hints.commands.map((cmd, i) => (
                <div key={i} className="group flex items-center gap-2 rounded bg-muted px-3 py-1.5">
                  <code className="flex-1 text-xs">{cmd}</code>
                  {!cmd.startsWith("#") && (
                    <button
                      onClick={() => copy(cmd)}
                      className="invisible text-muted-foreground transition-colors hover:text-foreground group-hover:visible"
                    >
                      <ClipboardCopyIcon className="size-3.5" />
                    </button>
                  )}
                </div>
              ))}
            </div>
            {hints.notes && (
              <p className="rounded border border-yellow-500/30 bg-yellow-500/5 px-3 py-2 text-xs text-yellow-700 dark:text-yellow-400">
                {hints.notes}
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// ---- AI Chat (uses existing task chat) ----
function SliversChatPanel({ configs }: { configs: SliverConfig[] }) {
  const [messages, setMessages] = React.useState<{ role: "user" | "assistant"; text: string }[]>([
    {
      role: "assistant",
      text: configs.length > 0
        ? `Sliver C2 assistant ready. ${configs.length} server(s) configured. I can help you generate commands, interpret output, manage beacons, and plan implant delivery. What do you need?`
        : "No Sliver C2 servers configured yet. Add a server in the Servers tab, then come back here. I can still help you plan implant delivery and generate commands to run in your Sliver console.",
    },
  ]);
  const [input, setInput] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const endRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  async function send() {
    const text = input.trim();
    if (!text) return;
    setMessages((m) => [...m, { role: "user", text }]);
    setInput("");
    setLoading(true);

    // Build context-aware hints via the hints endpoint
    try {
      const target = text.match(/\b(?:\d{1,3}\.){3}\d{1,3}\b/)?.[0] ?? "";
      const osMatch = text.match(/\b(windows|linux|macos|mac)\b/i)?.[1]?.toLowerCase() ?? "linux";
      let reply = "";

      if (target || text.toLowerCase().includes("implant") || text.toLowerCase().includes("beacon") || text.toLowerCase().includes("deploy")) {
        const hints = await api.sliverImplantHints({ target, os: osMatch });
        reply = `**Strategy:** ${hints.strategy}\n\n**Commands:**\n${hints.commands.map((c) => "```\n" + c + "\n```").join("\n")}\n\n**Notes:** ${hints.notes}`;
      } else if (text.toLowerCase().includes("list") || text.toLowerCase().includes("sessions") || text.toLowerCase().includes("beacons")) {
        reply = "To list active sessions in Sliver:\n```\nsliver > sessions\nsliver > beacons\n```\nTo interact with a session:\n```\nsliver > use <session-id>\nsliver [session] > info\nsliver [session] > ps\nsliver [session] > ls\n```";
      } else if (text.toLowerCase().includes("persist")) {
        reply = "Common persistence in Sliver:\n```\n# Windows\nsliver [session] > registry write --hive HKCU --key 'SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run' --value 'Updater' --data 'C:\\Windows\\Temp\\update.exe'\n# Linux\nsliver [session] > execute 'echo \"@reboot /tmp/.update\" | crontab -'\n```";
      } else if (text.toLowerCase().includes("screenshot") || text.toLowerCase().includes("keylog") || text.toLowerCase().includes("loot")) {
        reply = "Post-exploitation in Sliver:\n```\nsliver [session] > screenshot\nsliver [session] > armory install all\nsliver [session] > execute-assembly /path/to/SharpHound.exe\nsliver [session] > loot\nsliver [session] > upload /local/file /remote/path\nsliver [session] > download /remote/file /local/path\n```";
      } else {
        reply = `I'll help with that. Here's what I know about "${text}":\n\nFor specific Sliver commands, check: \`help\` in the Sliver console, or the [Sliver documentation](https://sliver.sh). For implant delivery hints, include a target IP or hostname in your message.`;
      }

      setMessages((m) => [...m, { role: "assistant", text: reply }]);
    } catch {
      setMessages((m) => [...m, { role: "assistant", text: "Failed to generate a response. Check your connection." }]);
    } finally { setLoading(false); }
  }

  return (
    <div className="flex h-[500px] flex-col gap-3">
      <div className="flex-1 overflow-y-auto space-y-3 rounded-lg border bg-muted/30 p-3">
        {messages.map((m, i) => (
          <div key={i} className={cn("flex", m.role === "user" ? "justify-end" : "justify-start")}>
            <div className={cn(
              "max-w-[80%] rounded-lg px-3 py-2 text-sm whitespace-pre-wrap",
              m.role === "user" ? "bg-primary text-primary-foreground" : "bg-background border",
            )}>
              {m.text}
            </div>
          </div>
        ))}
        {loading && (
          <div className="flex justify-start">
            <div className="rounded-lg border bg-background px-3 py-2 text-sm text-muted-foreground">Thinking...</div>
          </div>
        )}
        <div ref={endRef} />
      </div>
      <div className="flex gap-2">
        <Input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); } }}
          placeholder="Ask about implants, sessions, commands, or persistence…"
          disabled={loading}
        />
        <Button onClick={send} disabled={loading || !input.trim()}>Send</Button>
      </div>
    </div>
  );
}

// ---- Main page ----
export default function C2Page() {
  const [configs, setConfigs] = React.useState<SliverConfig[]>([]);
  const [loading, setLoading] = React.useState(true);

  const load = React.useCallback(() => {
    setLoading(true);
    api.listSliverConfigs().then(setConfigs).catch(() => {}).finally(() => setLoading(false));
  }, []);

  React.useEffect(() => { load(); }, [load]);

  async function deleteConfig(id: number) {
    try {
      await api.deleteSliverConfig(id);
      setConfigs((p) => p.filter((c) => c.id !== id));
      toast.success("Server config deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete");
    }
  }

  return (
    <div className="container max-w-5xl space-y-6 py-6">
      <div>
        <h1 className="text-xl font-semibold">Sliver C2</h1>
        <p className="text-sm text-muted-foreground">
          Manage Sliver C2 server configs and get AI-guided implant acquisition steps. ARTEX does not connect to Sliver directly — all commands are executed by you in the Sliver console.
        </p>
      </div>

      <Tabs defaultValue="chat">
        <TabsList>
          <TabsTrigger value="chat">AI Assistant</TabsTrigger>
          <TabsTrigger value="hints">Implant Hints</TabsTrigger>
          <TabsTrigger value="servers">Servers ({configs.length})</TabsTrigger>
        </TabsList>

        <TabsContent value="chat" className="pt-4">
          <SliversChatPanel configs={configs} />
        </TabsContent>

        <TabsContent value="hints" className="pt-4">
          <ImplantHintsPanel />
        </TabsContent>

        <TabsContent value="servers" className="pt-4 space-y-4">
          <div className="flex justify-end">
            <AddServerDialog onCreated={(c) => setConfigs((p) => [...p, c])} />
          </div>

          {loading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : configs.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center gap-2 py-10 text-center">
                <ServerIcon className="size-8 text-muted-foreground/40" />
                <p className="text-sm text-muted-foreground">No servers configured</p>
                <p className="text-xs text-muted-foreground">Add a Sliver server to store its connection details.</p>
              </CardContent>
            </Card>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Host</TableHead>
                  <TableHead>Port</TableHead>
                  <TableHead>Operator</TableHead>
                  <TableHead>Last seen</TableHead>
                  <TableHead className="w-10" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {configs.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-medium">
                      {c.name}
                      {c.enabled && <Badge variant="secondary" className="ml-2 text-xs">enabled</Badge>}
                    </TableCell>
                    <TableCell className="font-mono text-sm">{c.host}</TableCell>
                    <TableCell>{c.port}</TableCell>
                    <TableCell>{c.operator || "—"}</TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {c.last_seen ? new Date(c.last_seen).toLocaleString() : "Never"}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8 text-muted-foreground hover:text-destructive"
                        onClick={() => deleteConfig(c.id)}
                      >
                        <Trash2Icon className="size-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
