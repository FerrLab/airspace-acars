import { Card, CardContent } from "@/components/ui/card";

interface FlightDataCardProps {
  label: string;
  value: number | null;
  unit: string;
  decimals?: number;
}

export function FlightDataCard({ label, value, unit, decimals = 0 }: FlightDataCardProps) {
  return (
    <Card className="border-border/50">
      <CardContent className="p-4">
        <p className="text-xs font-medium text-muted-foreground tracking-tight">{label}</p>
        <div className="mt-1 flex items-baseline gap-1.5">
          <span className="text-2xl font-semibold tabular-nums tracking-tight">
            {value !== null ? value.toFixed(decimals) : "---"}
          </span>
          <span className="text-xs text-muted-foreground">{unit}</span>
        </div>
      </CardContent>
    </Card>
  );
}
