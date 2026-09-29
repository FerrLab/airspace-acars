import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { cn } from "@/lib/utils";
import { useTheme } from "../context/theme-context";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";

interface FlightsMapProps {
  flights: FlightLog[];
  selectedFlightId: string | null;
  onSelectFlight: (flightId: string) => void;
  className?: string;
  overlayContent?: React.ReactNode;
}

function escapeHtml(value: unknown): string {
  if (value == null) return "";
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

const ARCGIS_DARK_BASE =
  "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Base/MapServer/tile/{z}/{y}/{x}";
const ARCGIS_DARK_REF =
  "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Dark_Gray_Reference/MapServer/tile/{z}/{y}/{x}";

const ARCGIS_LIGHT_BASE =
  "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Light_Gray_Base/MapServer/tile/{z}/{y}/{x}";
const ARCGIS_LIGHT_REF =
  "https://server.arcgisonline.com/ArcGIS/rest/services/Canvas/World_Light_Gray_Reference/MapServer/tile/{z}/{y}/{x}";

// Generate intermediate arc points between two coordinates for flight-like curvature
function calculateArcCoordinates(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
  numPoints = 28
): [number, number][] {
  if ((lat1 === lat2 && lon1 === lon2) || (!lat1 && !lon1) || (!lat2 && !lon2)) {
    return [[lat1, lon1]];
  }

  const dLat = lat2 - lat1;
  const dLon = lon2 - lon1;
  const distance = Math.hypot(dLat, dLon);

  // Curve height proportional to distance, with a reasonable cap
  const maxArcHeight = Math.min(distance * 0.15, 6.0);

  // Normal vector perpendicular to the line connecting points
  const normalLat = -dLon / (distance || 1);
  const normalLon = dLat / (distance || 1);

  const points: [number, number][] = [];
  for (let i = 0; i <= numPoints; i++) {
    const t = i / numPoints;
    // Parabolic arc displacement
    const arcOffset = 4 * t * (1 - t) * maxArcHeight;

    const lat = lat1 + dLat * t + normalLat * arcOffset;
    const lon = lon1 + dLon * t + normalLon * arcOffset;
    points.push([lat, lon]);
  }
  return points;
}

function createAirportIcon(
  icao: string,
  isOrigin: boolean,
  isSelected: boolean,
  isDark: boolean
) {
  const safeIcao = escapeHtml(icao);
  const pulseClass = isSelected
    ? isDark
      ? "bg-sky-400/40 animate-ping"
      : "bg-sky-500/40 animate-ping"
    : isOrigin
      ? isDark
        ? "bg-emerald-400/30"
        : "bg-emerald-500/25"
      : isDark
        ? "bg-amber-400/30"
        : "bg-amber-500/25";

  const pinClass = isOrigin
    ? isDark
      ? "bg-emerald-500 text-white border-2 border-emerald-200 shadow-md shadow-emerald-500/50 scale-105"
      : "bg-emerald-600 text-white border-2 border-white shadow-md shadow-emerald-700/40 scale-105"
    : isDark
      ? "bg-amber-500 text-black border-2 border-amber-200 shadow-md shadow-amber-500/50 scale-105"
      : "bg-amber-500 text-white border-2 border-white shadow-md shadow-amber-600/40 scale-105";

  const labelClass = isOrigin
    ? isDark
      ? "bg-emerald-950/90 text-emerald-300 border border-emerald-500/50 shadow-md"
      : "bg-white/95 text-emerald-800 border border-emerald-300 shadow-sm font-bold"
    : isDark
      ? "bg-amber-950/90 text-amber-300 border border-amber-500/50 shadow-md"
      : "bg-white/95 text-amber-900 border border-amber-300 shadow-sm font-bold";

  return L.divIcon({
    className: "custom-airport-marker",
    html: `
      <div class="relative flex items-center justify-center cursor-pointer group">
        <div class="absolute -inset-2 rounded-full ${pulseClass} transition-all"></div>
        <div class="relative flex h-5 w-5 items-center justify-center rounded-full ${pinClass} transition-transform group-hover:scale-115">
          <div class="h-1.5 w-1.5 rounded-full bg-white"></div>
        </div>
        <span class="absolute top-5 left-1/2 -translate-x-1/2 whitespace-nowrap rounded px-1.5 py-0.5 text-[10px] font-mono tracking-wider ${labelClass} transition-all">
          ${safeIcao}
        </span>
      </div>
    `,
    iconSize: [24, 24],
    iconAnchor: [12, 12],
    popupAnchor: [0, -14],
  });
}

export function FlightsMap({
  flights,
  selectedFlightId,
  onSelectFlight,
  className = "",
  overlayContent,
}: FlightsMapProps) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const isDark = theme === "dark";

  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const layersGroupRef = useRef<L.FeatureGroup | null>(null);
  const tileLayersRef = useRef<{ base: L.TileLayer; ref: L.TileLayer } | null>(null);

  // Initialize Map instance
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    try {
      const map = L.map(containerRef.current, {
        zoomControl: false,
        attributionControl: false,
        minZoom: 2,
        maxZoom: 16,
      }).setView([-14.235, -51.9253], 4); // Centered over South America / Brazil by default

      // Add zoom control at bottom right
      L.control.zoom({ position: "bottomright" }).addTo(map);

      // Clean ArcGIS Base layer (dynamically supports Dark Gray and Light Gray)
      const baseLayer = L.tileLayer(
        isDark ? ARCGIS_DARK_BASE : ARCGIS_LIGHT_BASE,
        {
          maxZoom: 16,
        }
      ).addTo(map);

      // Clean ArcGIS Reference layer (borders, coastlines, labels)
      const refLayer = L.tileLayer(
        isDark ? ARCGIS_DARK_REF : ARCGIS_LIGHT_REF,
        {
          maxZoom: 16,
          opacity: isDark ? 0.8 : 0.9,
        }
      ).addTo(map);

      tileLayersRef.current = { base: baseLayer, ref: refLayer };

      const layersGroup = L.featureGroup().addTo(map);
      layersGroupRef.current = layersGroup;
      mapRef.current = map;

      // Invalidate size on container resize
      const observer = new ResizeObserver(() => {
        map.invalidateSize();
      });
      observer.observe(containerRef.current);

      // Post-mount resize trigger to ensure bounds settle accurately
      const resizeTimer = setTimeout(() => {
        if (mapRef.current) {
          map.invalidateSize();
        }
      }, 100);

      return () => {
        clearTimeout(resizeTimer);
        observer.disconnect();
        map.remove();
        mapRef.current = null;
        layersGroupRef.current = null;
        tileLayersRef.current = null;
      };
    } catch {
      // Graceful fallback for test runners / environments without Canvas/WebGL
    }
  }, []);

  // Dynamically switch basemap tiles when theme toggles
  useEffect(() => {
    if (!tileLayersRef.current) return;
    tileLayersRef.current.base.setUrl(isDark ? ARCGIS_DARK_BASE : ARCGIS_LIGHT_BASE);
    tileLayersRef.current.ref.setUrl(isDark ? ARCGIS_DARK_REF : ARCGIS_LIGHT_REF);
  }, [isDark]);

  const isSingleFlight = flights.length === 1;

  // Update routes, polylines, and markers when flights, selection, or theme change
  useEffect(() => {
    const map = mapRef.current;
    const layersGroup = layersGroupRef.current;
    if (!map || !layersGroup) return;

    layersGroup.clearLayers();

    const airportMap = new Map<
      string,
      {
        icao: string;
        name: string;
        city: string;
        lat: number;
        lon: number;
        isOrigin: boolean;
      }
    >();

    let selectedFlightBounds: L.LatLngBounds | null = null;
    const validFlightsWithCoords: FlightLog[] = [];

    flights.forEach((flight) => {
      const dep = flight.departure_airport;
      const arr = flight.arrival_airport;

      if (!dep?.latitude || !dep?.longitude || !arr?.latitude || !arr?.longitude) {
        return;
      }

      validFlightsWithCoords.push(flight);
      const isSelected = flight.id === selectedFlightId;

      if (!airportMap.has(dep.icao)) {
        airportMap.set(dep.icao, {
          icao: dep.icao,
          name: dep.name || dep.city || dep.icao,
          city: dep.city || "",
          lat: dep.latitude,
          lon: dep.longitude,
          isOrigin: true,
        });
      }
      if (!airportMap.has(arr.icao)) {
        airportMap.set(arr.icao, {
          icao: arr.icao,
          name: arr.name || arr.city || arr.icao,
          city: arr.city || "",
          lat: arr.latitude,
          lon: arr.longitude,
          isOrigin: false,
        });
      }

      // Draw curved flight path
      const arcPoints = calculateArcCoordinates(
        dep.latitude,
        dep.longitude,
        arr.latitude,
        arr.longitude
      );

      // Primary Route line with theme-aware colors
      const normalColor = isDark ? "#0284c7" : "#475569";
      const activeColor = isDark ? "#38bdf8" : "#0284c7";

      const polyline = L.polyline(arcPoints, {
        color: isSelected ? activeColor : normalColor,
        weight: isSelected ? 4 : 2,
        opacity: isSelected ? 0.95 : isDark ? 0.45 : 0.4,
        dashArray: isSelected ? undefined : "6, 8",
        lineCap: "round",
        lineJoin: "round",
      });

      // Hover / Click behavior on route
      polyline.on("mouseover", () => {
        if (!isSelected) {
          polyline.setStyle({ color: activeColor, weight: 3.5, opacity: 0.85 });
        }
      });
      polyline.on("mouseout", () => {
        if (!isSelected) {
          polyline.setStyle({ color: normalColor, weight: 2, opacity: isDark ? 0.45 : 0.4 });
        }
      });
      polyline.on("click", () => {
        onSelectFlight(flight.id);
      });

      const safeCallsign = escapeHtml(flight.callsign);
      const safeAircraft = escapeHtml(flight.aircraft?.icao_code || "A/C");
      const safeDepIcao = escapeHtml(dep.icao);
      const safeArrIcao = escapeHtml(arr.icao);
      const distLabel = escapeHtml(t("myFlights.distance", "Dist"));
      const timeLabel = escapeHtml(t("myFlights.flightTime", "Tempo"));
      const landingLabel = escapeHtml(t("myFlights.landingRate", "Toque"));
      const scoreLabel = escapeHtml(t("myFlights.score", "Score"));
      const scoreDisplay =
        flight.score != null && flight.score > 0 ? `${flight.score} pts` : "—";

      const popupContent = isDark
        ? `
        <div class="text-xs p-2 text-slate-100 font-sans space-y-1.5 min-w-[170px]">
          <div class="font-bold text-sm text-sky-400 flex items-center justify-between gap-3">
            <span>${safeCallsign}</span>
            <span class="text-[10px] px-1.5 py-0.5 rounded bg-sky-950/80 text-sky-300 border border-sky-800/60 font-mono">${safeAircraft}</span>
          </div>
          <div class="text-[11px] text-slate-300 font-medium">
            ${safeDepIcao} ➔ ${safeArrIcao}
          </div>
          <div class="grid grid-cols-2 gap-x-2 gap-y-1 text-[10px] text-slate-400 pt-1.5 border-t border-slate-700/60">
            <div>${distLabel}: <span class="text-slate-100 font-medium">${Math.round(flight.distance_nm)} NM</span></div>
            <div>${timeLabel}: <span class="text-slate-100 font-medium">${flight.flight_time_minutes}m</span></div>
            <div>${landingLabel}: <span class="text-emerald-400 font-medium">${flight.landing_rate_fpm ? `${Math.round(flight.landing_rate_fpm)}` : "—"} fpm</span></div>
            <div>${scoreLabel}: <span class="text-slate-100 font-medium">${scoreDisplay}</span></div>
          </div>
        </div>
      `
        : `
        <div class="text-xs p-2 text-slate-900 font-sans space-y-1.5 min-w-[170px]">
          <div class="font-bold text-sm text-sky-700 flex items-center justify-between gap-3">
            <span>${safeCallsign}</span>
            <span class="text-[10px] px-1.5 py-0.5 rounded bg-sky-100 text-sky-800 border border-sky-300 font-mono">${safeAircraft}</span>
          </div>
          <div class="text-[11px] text-slate-700 font-medium">
            ${safeDepIcao} ➔ ${safeArrIcao}
          </div>
          <div class="grid grid-cols-2 gap-x-2 gap-y-1 text-[10px] text-slate-500 pt-1.5 border-t border-slate-200">
            <div>${distLabel}: <span class="text-slate-900 font-medium">${Math.round(flight.distance_nm)} NM</span></div>
            <div>${timeLabel}: <span class="text-slate-900 font-medium">${flight.flight_time_minutes}m</span></div>
            <div>${landingLabel}: <span class="text-emerald-700 font-medium">${flight.landing_rate_fpm ? `${Math.round(flight.landing_rate_fpm)}` : "—"} fpm</span></div>
            <div>${scoreLabel}: <span class="text-slate-900 font-medium">${scoreDisplay}</span></div>
          </div>
        </div>
      `;
      polyline.bindPopup(popupContent, { closeButton: false });

      layersGroup.addLayer(polyline);

      if (isSelected) {
        selectedFlightBounds = L.latLngBounds([
          [dep.latitude, dep.longitude],
          [arr.latitude, arr.longitude],
        ]);
      }
    });

    // Render airport markers
    airportMap.forEach((airport) => {
      const isSelectedAirport =
        isSingleFlight ||
        (Boolean(selectedFlightId) &&
          flights.some(
            (f) =>
              f.id === selectedFlightId &&
              (f.departure_airport?.icao === airport.icao || f.arrival_airport?.icao === airport.icao)
          ));

      const marker = L.marker([airport.lat, airport.lon], {
        icon: createAirportIcon(airport.icao, airport.isOrigin, isSelectedAirport, isDark),
        zIndexOffset: isSelectedAirport ? 1000 : 100,
      });

      const safeAirportIcao = escapeHtml(airport.icao);
      const safeAirportName = escapeHtml(airport.name);
      const safeAirportCity = escapeHtml(airport.city);

      const airportPopupContent = isDark
        ? `
        <div class="text-xs p-2 text-slate-100 font-sans min-w-[140px]">
          <div class="font-bold text-sky-400 text-sm font-mono">${safeAirportIcao}</div>
          <div class="text-[11px] text-slate-200 font-medium mt-0.5">${safeAirportName}</div>
          ${safeAirportCity ? `<div class="text-[10px] text-slate-400 mt-0.5">${safeAirportCity}</div>` : ""}
        </div>
        `
        : `
        <div class="text-xs p-2 text-slate-900 font-sans min-w-[140px]">
          <div class="font-bold text-sky-700 text-sm font-mono">${safeAirportIcao}</div>
          <div class="text-[11px] text-slate-800 font-medium mt-0.5">${safeAirportName}</div>
          ${safeAirportCity ? `<div class="text-[10px] text-slate-500 mt-0.5">${safeAirportCity}</div>` : ""}
        </div>
        `;

      marker.bindPopup(airportPopupContent, { closeButton: false });

      layersGroup.addLayer(marker);
    });

    // Fit view bounds with generous padding so the route breathes
    if (selectedFlightBounds) {
      const bounds: L.LatLngBounds = selectedFlightBounds;
      setTimeout(() => {
        if (mapRef.current) {
          mapRef.current.invalidateSize();
          mapRef.current.fitBounds(bounds, {
            padding: [70, 70],
            maxZoom: 7,
            animate: true,
          });
        }
      }, 100);
    } else if (validFlightsWithCoords.length > 0) {
      const allBounds = layersGroup.getBounds();
      if (allBounds.isValid()) {
        setTimeout(() => {
          if (mapRef.current) {
            mapRef.current.invalidateSize();
            mapRef.current.fitBounds(allBounds, {
              padding: [45, 45],
              maxZoom: 7,
              animate: true,
            });
          }
        }, 100);
      }
    }
  }, [flights, selectedFlightId, onSelectFlight, isDark, isSingleFlight, t]);

  const handleResetBounds = () => {
    if (!mapRef.current || !layersGroupRef.current) return;
    const bounds = layersGroupRef.current.getBounds();
    if (bounds.isValid()) {
      mapRef.current.fitBounds(bounds, { padding: [50, 50], animate: true });
    }
  };

  return (
    <div className={cn("relative h-full w-full overflow-hidden bg-card", className)}>
      <div
        ref={containerRef}
        className={cn(
          "h-full w-full transition-colors duration-300",
          isDark ? "bg-[#111622]" : "bg-[#f8fafc]"
        )}
      />

      {/* Floating Header / Control Bar Overlay */}
      {overlayContent ? (
        <div className="absolute top-3 left-3 z-[400] flex items-center gap-2 rounded-lg border border-border/70 bg-card/90 px-3 py-1.5 shadow-md backdrop-blur-md text-xs text-card-foreground">
          {overlayContent}
        </div>
      ) : !isSingleFlight ? (
        <div className="absolute top-3 left-3 z-[400] flex items-center gap-2 rounded-lg border border-border/70 bg-card/90 px-3 py-1.5 shadow-md backdrop-blur-md text-xs text-card-foreground">
          <span className="flex h-2 w-2 rounded-full bg-sky-500 animate-pulse" />
          <span className="font-medium text-foreground">
            {flights.length}{" "}
            {flights.length === 1
              ? t("myFlights.mapFlightMapped", "Voo mapeado")
              : t("myFlights.mapFlightsMapped", "Voos mapeados")}
          </span>
          <button
            type="button"
            onClick={handleResetBounds}
            className="ml-2 rounded px-2 py-0.5 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground transition-colors border border-border/60"
          >
            {t("myFlights.mapCenter", "Centralizar")}
          </button>
        </div>
      ) : null}

      {flights.length === 0 && (
        <div className="absolute inset-0 z-[400] flex flex-col items-center justify-center bg-background/50 backdrop-blur-xs text-center p-4">
          <p className="text-sm font-medium text-muted-foreground">
            {t(
              "myFlights.mapNoCoords",
              "Nenhum voo recente com coordenadas disponíveis para exibição no mapa."
            )}
          </p>
        </div>
      )}
    </div>
  );
}
