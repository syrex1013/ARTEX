"use client";

import * as React from "react";

import {
  ChevronRightIcon,
  MailIcon,
  MessageSquareIcon,
  PhoneCallIcon,
  PlusIcon,
  Trash2Icon,
  UserPlusIcon,
  UsersIcon,
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
import type { SETemplate, SocialEngCampaign, SocialEngTarget } from "@/lib/types";
import { cn } from "@/lib/utils";

const KIND_ICON: Record<string, React.ElementType> = {
  phishing: MailIcon,
  smishing: MessageSquareIcon,
  vishing: PhoneCallIcon,
  pretexting: UsersIcon,
};

const STATUS_COLORS: Record<string, string> = {
  planning: "bg-muted text-muted-foreground",
  active: "bg-green-500/15 text-green-700 dark:text-green-400",
  paused: "bg-yellow-500/15 text-yellow-700 dark:text-yellow-400",
  completed: "bg-blue-500/15 text-blue-700 dark:text-blue-400",
};

function CampaignBadge({ status }: { status: string }) {
  return (
    <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium", STATUS_COLORS[status] ?? "bg-muted")}>
      {status}
    </span>
  );
}

function StatBadge({ label, value }: { label: string; value: number }) {
  if (!value) return null;
  return (
    <span className="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </span>
  );
}

// ---- Create Campaign Dialog ----
function CreateCampaignDialog({ onCreated }: { onCreated: (c: SocialEngCampaign) => void }) {
  const [open, setOpen] = React.useState(false);
  const [name, setName] = React.useState("");
  const [kind, setKind] = React.useState("phishing");
  const [objective, setObjective] = React.useState("");
  const [templateName, setTemplateName] = React.useState("");
  const [redirectUrl, setRedirectUrl] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  async function submit() {
    if (!name) { toast.error("Name is required"); return; }
    setSaving(true);
    try {
      const c = await api.createCampaign({ name, kind: kind as SocialEngCampaign["kind"], objective, template_name: templateName, redirect_url: redirectUrl });
      onCreated(c);
      setOpen(false);
      setName(""); setKind("phishing"); setObjective(""); setTemplateName(""); setRedirectUrl("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create campaign");
    } finally { setSaving(false); }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm"><PlusIcon className="mr-1.5 size-4" />New Campaign</Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>New Campaign</DialogTitle>
          <DialogDescription>Create a social engineering campaign. Authorized engagements only.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-1.5">
            <Label>Campaign name</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Q1 Phishing Test" />
          </div>
          <div className="grid gap-1.5">
            <Label>Kind</Label>
            <Select value={kind} onValueChange={setKind}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {["phishing","vishing","smishing","pretexting"].map((k) => (
                  <SelectItem key={k} value={k}>{k.charAt(0).toUpperCase() + k.slice(1)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label>Objective</Label>
            <Textarea value={objective} onChange={(e) => setObjective(e.target.value)} placeholder="Test employee susceptibility to credential phishing" rows={2} />
          </div>
          <div className="grid gap-1.5">
            <Label>Template name (optional)</Label>
            <Input value={templateName} onChange={(e) => setTemplateName(e.target.value)} placeholder="IT Password Reset" />
          </div>
          <div className="grid gap-1.5">
            <Label>Redirect URL (after click, optional)</Label>
            <Input value={redirectUrl} onChange={(e) => setRedirectUrl(e.target.value)} placeholder="https://..." />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={submit} disabled={saving}>{saving ? "Creating..." : "Create"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---- Add Target Dialog ----
function AddTargetDialog({ campaignId, onAdded }: { campaignId: number; onAdded: () => void }) {
  const [open, setOpen] = React.useState(false);
  const [email, setEmail] = React.useState("");
  const [phone, setPhone] = React.useState("");
  const [name, setName] = React.useState("");
  const [dept, setDept] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  async function submit() {
    setSaving(true);
    try {
      await api.addCampaignTarget(campaignId, { email, phone, name, department: dept });
      onAdded();
      setOpen(false);
      setEmail(""); setPhone(""); setName(""); setDept("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add target");
    } finally { setSaving(false); }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline"><UserPlusIcon className="mr-1.5 size-4" />Add Target</Button>
      </DialogTrigger>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Add Target</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3 py-2">
          <div className="grid gap-1.5">
            <Label>Name</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Jane Smith" />
          </div>
          <div className="grid gap-1.5">
            <Label>Email</Label>
            <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="jane@example.com" type="email" />
          </div>
          <div className="grid gap-1.5">
            <Label>Phone (vishing/smishing)</Label>
            <Input value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+1 555 0100" />
          </div>
          <div className="grid gap-1.5">
            <Label>Department</Label>
            <Input value={dept} onChange={(e) => setDept(e.target.value)} placeholder="Finance" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={submit} disabled={saving}>{saving ? "Adding..." : "Add"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---- Detail panel ----
function CampaignDetail({ campaign, onStatusChange }: { campaign: SocialEngCampaign; onStatusChange: () => void }) {
  const [targets, setTargets] = React.useState<SocialEngTarget[]>([]);
  const [templates, setTemplates] = React.useState<SETemplate[]>([]);
  const [loadingTargets, setLoadingTargets] = React.useState(true);
  const [updatingStatus, setUpdatingStatus] = React.useState(false);

  const loadTargets = React.useCallback(() => {
    setLoadingTargets(true);
    api.listCampaignTargets(campaign.id).then(setTargets).catch(() => {}).finally(() => setLoadingTargets(false));
  }, [campaign.id]);

  React.useEffect(() => {
    loadTargets();
    api.seTemplates().then(setTemplates).catch(() => {});
  }, [loadTargets]);

  async function setStatus(status: string) {
    setUpdatingStatus(true);
    try {
      await api.patchCampaign(campaign.id, { status } as Partial<import("@/lib/types").SocialEngCampaign>);
      onStatusChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to update status");
    } finally { setUpdatingStatus(false); }
  }

  const trackBase = `/api/social-eng/track?t=TARGET_ID&e=opened`;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold">{campaign.name}</h2>
            <CampaignBadge status={campaign.status} />
          </div>
          {campaign.objective && <p className="text-sm text-muted-foreground">{campaign.objective}</p>}
        </div>
        <div className="flex gap-2">
          {campaign.status !== "active" && (
            <Button size="sm" onClick={() => setStatus("active")} disabled={updatingStatus}>Activate</Button>
          )}
          {campaign.status === "active" && (
            <Button size="sm" variant="outline" onClick={() => setStatus("paused")} disabled={updatingStatus}>Pause</Button>
          )}
          {campaign.status !== "completed" && (
            <Button size="sm" variant="outline" onClick={() => setStatus("completed")} disabled={updatingStatus}>Complete</Button>
          )}
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {[
          { label: "Targets", value: campaign.total_targets },
          { label: "Sent", value: campaign.sent_count },
          { label: "Opened", value: campaign.opened_count },
          { label: "Clicked", value: campaign.clicked_count },
          { label: "Submitted", value: campaign.submitted_count },
        ].map(({ label, value }) => (
          <Card key={label} className="p-3">
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className="text-2xl font-semibold">{value}</p>
          </Card>
        ))}
      </div>

      <Tabs defaultValue="targets">
        <TabsList>
          <TabsTrigger value="targets">Targets ({targets.length})</TabsTrigger>
          <TabsTrigger value="templates">Templates</TabsTrigger>
          <TabsTrigger value="tracking">Tracking pixel</TabsTrigger>
        </TabsList>

        <TabsContent value="targets" className="space-y-3 pt-3">
          <div className="flex justify-end">
            <AddTargetDialog campaignId={campaign.id} onAdded={loadTargets} />
          </div>
          {loadingTargets ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : targets.length === 0 ? (
            <p className="text-sm text-muted-foreground">No targets yet. Add targets to get started.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Dept</TableHead>
                  <TableHead>Sent</TableHead>
                  <TableHead>Opened</TableHead>
                  <TableHead>Clicked</TableHead>
                  <TableHead>Submitted</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {targets.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell className="font-medium">{t.name || "—"}</TableCell>
                    <TableCell className="text-muted-foreground">{t.email || "—"}</TableCell>
                    <TableCell>{t.department || "—"}</TableCell>
                    <TableCell>{t.sent_at ? <Badge variant="secondary">✓</Badge> : "—"}</TableCell>
                    <TableCell>{t.opened_at ? <Badge variant="secondary">✓</Badge> : "—"}</TableCell>
                    <TableCell>{t.clicked_at ? <Badge variant="secondary">✓</Badge> : "—"}</TableCell>
                    <TableCell>{t.submitted_at ? <Badge variant="secondary">✓</Badge> : "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </TabsContent>

        <TabsContent value="templates" className="space-y-3 pt-3">
          {templates.length === 0 ? (
            <p className="text-sm text-muted-foreground">Loading templates...</p>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {templates.map((tpl) => (
                <Card key={tpl.name} className="overflow-hidden">
                  <CardHeader className="pb-2">
                    <div className="flex items-center justify-between gap-2">
                      <CardTitle className="text-sm">{tpl.name}</CardTitle>
                      <Badge variant="outline">{tpl.kind}</Badge>
                    </div>
                    {tpl.subject && <CardDescription className="text-xs">{tpl.subject}</CardDescription>}
                  </CardHeader>
                  <CardContent className="pb-3">
                    <pre className="whitespace-pre-wrap rounded bg-muted p-2 text-xs">{tpl.text_body || tpl.html_body}</pre>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="tracking" className="space-y-3 pt-3">
          <Card>
            <CardContent className="pt-4 space-y-3">
              <p className="text-sm">Embed this 1×1 tracking pixel in your phishing email to record when targets open it. Replace <code className="rounded bg-muted px-1 text-xs">TARGET_ID</code> with the numeric target ID and <code className="rounded bg-muted px-1 text-xs">opened</code> with the event (<code className="rounded bg-muted px-1 text-xs">opened</code>, <code className="rounded bg-muted px-1 text-xs">clicked</code>, <code className="rounded bg-muted px-1 text-xs">submitted</code>).</p>
              <pre className="rounded bg-muted p-3 text-xs overflow-x-auto">
{`<img src="${typeof window !== "undefined" ? window.location.origin : ""}/api/social-eng/track?t=TARGET_ID&e=opened" width="1" height="1" />`}
              </pre>
              <p className="text-xs text-muted-foreground">The pixel endpoint returns a transparent PNG and records the event without redirecting. For link tracking, link directly to the redirect URL and record <code className="rounded bg-muted px-1 text-xs">clicked</code> server-side.</p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

// ---- Main page ----
export default function SocialEngPage() {
  const [campaigns, setCampaigns] = React.useState<SocialEngCampaign[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [selected, setSelected] = React.useState<SocialEngCampaign | null>(null);

  const load = React.useCallback(() => {
    setLoading(true);
    api.listCampaigns().then(setCampaigns).catch(() => {}).finally(() => setLoading(false));
  }, []);

  React.useEffect(() => { load(); }, [load]);

  const refreshSelected = React.useCallback(() => {
    if (!selected) return;
    api.getCampaign(selected.id).then((c) => {
      setSelected(c);
      setCampaigns((prev) => prev.map((x) => x.id === c.id ? c : x));
    }).catch(() => {});
  }, [selected]);

  async function deleteCampaign(id: number) {
    try {
      await api.deleteCampaign(id);
      setCampaigns((prev) => prev.filter((c) => c.id !== id));
      if (selected?.id === id) setSelected(null);
      toast.success("Campaign deleted");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete");
    }
  }

  return (
    <div className="container max-w-7xl space-y-6 py-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold">Social Engineering</h1>
          <p className="text-sm text-muted-foreground">Manage phishing, vishing, smishing, and pretexting campaigns.</p>
        </div>
        <CreateCampaignDialog onCreated={(c) => { setCampaigns((p) => [c, ...p]); setSelected(c); }} />
      </div>

      <div className="grid gap-6 lg:grid-cols-[320px_1fr]">
        {/* Campaign list */}
        <div className="space-y-2">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : campaigns.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center gap-2 py-8 text-center">
                <MailIcon className="size-8 text-muted-foreground/40" />
                <p className="text-sm text-muted-foreground">No campaigns yet</p>
              </CardContent>
            </Card>
          ) : (
            campaigns.map((c) => {
              const KindIcon = KIND_ICON[c.kind] ?? MailIcon;
              return (
                <button
                  key={c.id}
                  onClick={() => setSelected(c)}
                  className={cn(
                    "flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent",
                    selected?.id === c.id && "border-primary bg-accent",
                  )}
                >
                  <KindIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center justify-between gap-2">
                      <p className="truncate text-sm font-medium">{c.name}</p>
                      <CampaignBadge status={c.status} />
                    </div>
                    <div className="flex flex-wrap gap-1">
                      <StatBadge label="targets" value={c.total_targets} />
                      <StatBadge label="sent" value={c.sent_count} />
                      <StatBadge label="clicked" value={c.clicked_count} />
                    </div>
                  </div>
                  <ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                </button>
              );
            })
          )}
        </div>

        {/* Detail panel */}
        <div>
          {selected ? (
            <CampaignDetail
              campaign={selected}
              onStatusChange={refreshSelected}
            />
          ) : (
            <Card>
              <CardContent className="flex flex-col items-center gap-2 py-16 text-center">
                <MailIcon className="size-10 text-muted-foreground/30" />
                <p className="text-sm text-muted-foreground">Select a campaign to view details</p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
