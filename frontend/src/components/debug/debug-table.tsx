import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";

export interface DataRow {
  label: string;
  value: ReactNode;
  unit?: string;
}

export function BoolBadge({ value }: { value: boolean }) {
  const { t } = useTranslation();
  return (
    <Badge variant={value ? "default" : "secondary"} className="text-[10px] px-1.5 py-0">
      {value ? t("debug.on") : t("debug.off")}
    </Badge>
  );
}

export function DataTable({ rows }: { rows: DataRow[] }) {
  return (
    <div className="rounded-md border border-border">
      <table className="w-full text-sm">
        <tbody>
          {rows.map((r) => (
            <tr key={r.label} className="border-b border-border/50 last:border-0">
              <td className="px-3 py-1 font-mono text-xs text-muted-foreground w-[160px]">{r.label}</td>
              <td className="px-3 py-1 text-right font-mono text-xs tabular-nums">{r.value}</td>
              {r.unit && <td className="px-2 py-1 text-xs text-muted-foreground w-[50px]">{r.unit}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function fmt(v: number, d = 2): string {
  return v.toFixed(d);
}

export function SectionTitle({ children }: { children: ReactNode }) {
  return <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{children}</h3>;
}
