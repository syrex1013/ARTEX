"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { http } from "@/lib/api";
import { engagementPath, mutation, type SnapshotFinding } from "@/lib/collaboration";

export function FindingEvidenceEditor({
  engagementId,
  findingId,
  editable,
  onSaved,
}: {
  engagementId: number;
  findingId: number;
  editable: boolean;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [finding, setFinding] = useState<SnapshotFinding | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const path = `${engagementPath(engagementId)}/findings/${findingId}`;
  async function load() {
    setOpen(true);
    setError("");
    setBusy(true);
    try {
      setFinding(await http<SnapshotFinding>(path));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!finding) return;
    setBusy(true);
    setError("");
    try {
      await mutation(path, "PATCH", { status: finding.status, summary: finding.summary, evidence: finding.evidence });
      onSaved();
      setOpen(false);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => void load()}>
        Evidence and disposition
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Finding #{findingId} evidence</DialogTitle>
          </DialogHeader>
          {error && (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          )}
          {busy && <p role="status">Loading / saving…</p>}
          {finding && (
            <div className="space-y-3">
              <p className="text-sm">
                Severity: {finding.severity} · Verification: {finding.verification_status} · Evidence version:{" "}
                {finding.evidence_version}
              </p>
              <select
                aria-label="Finding disposition"
                disabled={!editable || busy}
                className="rounded-md border bg-background p-2"
                value={finding.status}
                onChange={(e) => setFinding({ ...finding, status: e.target.value })}
              >
                {[
                  "pending",
                  "in_progress",
                  "confirmed",
                  "resolved",
                  "fixed",
                  "false_positive",
                  "ignored",
                  "duplicate",
                  "risk_accepted",
                ].map((status) => (
                  <option key={status}>{status}</option>
                ))}
              </select>
              <Textarea
                aria-label="Finding summary"
                readOnly={!editable}
                value={finding.summary}
                onChange={(e) => setFinding({ ...finding, summary: e.target.value })}
                maxLength={100000}
              />
              <Textarea
                aria-label="Finding evidence"
                readOnly={!editable}
                value={finding.evidence}
                onChange={(e) => setFinding({ ...finding, evidence: e.target.value })}
                maxLength={1000000}
              />
              {editable && (
                <Button disabled={busy} onClick={() => void save()}>
                  Save evidence
                </Button>
              )}
              <p className="text-muted-foreground text-xs">
                Evidence changes require fresh verification and mark existing report snapshots stale.
              </p>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
