/**
 * formatUtcDate shows a server timestamp as day/month and Zulu time
 * ("08/10 14:36z"), the way the logbook list and the flight detail both show
 * it. A missing or unparseable value is a dash.
 */
export function formatUtcDate(val?: string): string {
  if (!val) return "—";
  const d = new Date(val);
  if (Number.isNaN(d.getTime())) return "—";
  const day = String(d.getUTCDate()).padStart(2, "0");
  const month = String(d.getUTCMonth() + 1).padStart(2, "0");
  const hours = String(d.getUTCHours()).padStart(2, "0");
  const mins = String(d.getUTCMinutes()).padStart(2, "0");
  return `${day}/${month} ${hours}:${mins}z`;
}
