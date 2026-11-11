"use client";

import * as React from "react";

import {
  ChevronDownIcon,
  GlobeIcon,
  LoaderIcon,
  LockIcon,
  LockOpenIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  WifiIcon,
  WifiOffIcon,
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
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { api } from "@/lib/api";
import type { VPNRelay, VPNStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

function StatusIcon({ state }: { state: string }) {
  if (state === "Connected") return <ShieldCheckIcon className="size-5 text-green-500" />;
  if (state === "Connecting" || state === "Disconnecting") return <LoaderIcon className="size-5 animate-spin text-yellow-500" />;
  return <WifiOffIcon className="size-5 text-muted-foreground" />;
}

export default function VPNPage() {
  const [status, setStatus] = React.useState<VPNStatus | null>(null);
  const [relays, setRelays] = React.useState<VPNRelay[]>([]);
  const [account, setAccount] = React.useState("");
  const [accountInput, setAccountInput] = React.useState("");
  const [savingAccount, setSavingAccount] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [connecting, setConnecting] = React.useState(false);
  const [selectedServer, setSelectedServer] = React.useState("");
  const [available, setAvailable] = React.useState(true);

  const refreshStatus = React.useCallback(async () => {
    try {
      const s = await api.vpnStatus();
      setStatus(s);
      if (s.error?.includes("not found") || s.error?.includes("unavailable")) {
        setAvailable(false);
      }
    } catch {
      setAvailable(false);
    }
  }, []);

  React.useEffect(() => {
    setLoading(true);
    Promise.all([
      refreshStatus(),
      api.vpnRelays().then(setRelays).catch(() => {}),
      api.vpnAccount().then((r) => { setAccount(r.account); setAccountInput(r.account); }).catch(() => {}),
    ]).finally(() => setLoading(false));
  }, [refreshStatus]);

  async function connect() {
    setConnecting(true);
    try {
      const s = await api.vpnConnect(selectedServer || undefined);
      setStatus(s);
      toast.success("VPN connecting…");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to connect");
    } finally { setConnecting(false); }
  }

  async function disconnect() {
    setConnecting(true);
    try {
      const s = await api.vpnDisconnect();
      setStatus(s);
      toast.success("VPN disconnecting…");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to disconnect");
    } finally { setConnecting(false); }
  }

  async function saveAccount() {
    setSavingAccount(true);
    try {
      await api.vpnSetAccount(accountInput);
      setAccount(accountInput);
      toast.success("Account number saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save account");
    } finally { setSavingAccount(false); }
  }

  const connected = status?.state === "Connected";

  return (
    <div className="container max-w-3xl space-y-6 py-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold">Mullvad VPN</h1>
          <p className="text-sm text-muted-foreground">
            Control Mullvad VPN on the ARTEX server. Requires the{" "}
            <code className="rounded bg-muted px-1 text-xs">mullvad</code> CLI to be installed server-side.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refreshStatus}>
          <RefreshCwIcon className="mr-1.5 size-4" />
          Refresh
        </Button>
      </div>

      {!available && (
        <Card className="border-yellow-500/40 bg-yellow-500/5">
          <CardContent className="pt-4 pb-3">
            <p className="text-sm text-yellow-700 dark:text-yellow-400">
              <strong>Mullvad CLI not found.</strong> Install Mullvad on the server and ensure the <code className="rounded bg-muted px-1 text-xs">mullvad</code> binary is in PATH.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Status Card */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <StatusIcon state={status?.state ?? ""} />
            {status?.state ?? (loading ? "Loading…" : "Unknown")}
          </CardTitle>
          {connected && status?.location && (
            <CardDescription>
              <GlobeIcon className="mr-1 inline-block size-3.5" />
              {status.location}
              {status.ip && <span className="ml-2 font-mono text-xs">{status.ip}</span>}
            </CardDescription>
          )}
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            {!connected ? (
              <Button onClick={connect} disabled={connecting || !available} className="gap-1.5">
                <LockIcon className="size-4" />
                {connecting ? "Connecting…" : "Connect"}
              </Button>
            ) : (
              <Button variant="outline" onClick={disconnect} disabled={connecting} className="gap-1.5">
                <LockOpenIcon className="size-4" />
                {connecting ? "Disconnecting…" : "Disconnect"}
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Server selection */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Server selection</CardTitle>
          <CardDescription>Leave blank to use the fastest available server, or specify a hostname.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex gap-2">
            <Input
              value={selectedServer}
              onChange={(e) => setSelectedServer(e.target.value)}
              placeholder="e.g. se-got-wg-001.mullvad.net"
              className="font-mono text-sm"
            />
            <Button onClick={connect} disabled={connecting || !available} size="sm">
              Connect
            </Button>
          </div>

          {/* Relay browser */}
          {relays.length > 0 && (
            <div className="space-y-1 max-h-80 overflow-y-auto rounded-lg border">
              {relays.map((country) => (
                <Collapsible key={country.country_code}>
                  <CollapsibleTrigger className="flex w-full items-center justify-between px-3 py-2 text-sm hover:bg-accent">
                    <span className="font-medium">{country.country}</span>
                    <ChevronDownIcon className="size-4 text-muted-foreground" />
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    {country.cities.map((city) => (
                      <div key={city.name} className="pl-4">
                        <p className="px-3 py-1 text-xs font-medium text-muted-foreground">{city.name}</p>
                        {city.servers.map((srv) => (
                          <button
                            key={srv.hostname}
                            onClick={() => setSelectedServer(srv.hostname)}
                            className={cn(
                              "flex w-full items-center justify-between px-3 py-1.5 text-xs hover:bg-accent",
                              selectedServer === srv.hostname && "bg-accent font-medium",
                            )}
                          >
                            <span className="font-mono">{srv.hostname}</span>
                            {srv.active ? (
                              <Badge variant="secondary" className="text-xs">active</Badge>
                            ) : (
                              <span className="text-muted-foreground/50">inactive</span>
                            )}
                          </button>
                        ))}
                      </div>
                    ))}
                  </CollapsibleContent>
                </Collapsible>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Account */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Mullvad account</CardTitle>
          <CardDescription>Your 16-digit Mullvad account number.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            <Input
              value={accountInput}
              onChange={(e) => setAccountInput(e.target.value)}
              placeholder="0000 0000 0000 0000"
              className="font-mono max-w-xs"
              type="password"
              autoComplete="off"
            />
            <Button
              onClick={saveAccount}
              disabled={savingAccount || accountInput === account}
              size="sm"
            >
              {savingAccount ? "Saving…" : "Save"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
