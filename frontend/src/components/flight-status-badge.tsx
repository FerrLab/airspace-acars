import { Badge } from "@/components/ui/badge";

export function normalizeFlightStatus(status?: string): string {
  if (!status || !status.trim()) return "unknown";
  return status.trim().toLowerCase();
}

// Shared by the logbook list and the flight detail, so a flight's status reads
// and is coloured the same in both.
export function getFlightStatusBadge(status: string | undefined, t: (key: string, fallback: string) => string) {
  const norm = normalizeFlightStatus(status);
  if (norm === "unknown") {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-border text-muted-foreground bg-muted/20 font-medium"
      >
        —
      </Badge>
    );
  }
  // The platform auto-accepts flights that raised FDM warnings under
  // warningSystem / warningStaff; they count as accepted.
  if (norm.includes("warning")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-amber-500/30 text-amber-400 bg-amber-500/10 font-medium"
      >
        {t("myFlights.statusWarning", "Aceito com aviso")}
      </Badge>
    );
  }
  if (norm.includes("accept") || norm.includes("approved") || norm.includes("closed")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-emerald-500/30 text-emerald-400 bg-emerald-500/10 font-medium"
      >
        {t("myFlights.statusApproved", "Aprovado")}
      </Badge>
    );
  }
  if (norm.includes("pending") || norm.includes("review")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-amber-500/30 text-amber-400 bg-amber-500/10 font-medium"
      >
        {t("myFlights.statusPending", "Pendente")}
      </Badge>
    );
  }
  if (norm.includes("reject") || norm.includes("denied")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-red-500/30 text-red-400 bg-red-500/10 font-medium"
      >
        {t("myFlights.statusRejected", "Rejeitado")}
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="text-[10px] py-0 px-2 border-red-500/30 text-red-400 bg-red-500/10 font-medium"
    >
      {status}
    </Badge>
  );
}
