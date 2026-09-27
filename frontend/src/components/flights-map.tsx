import { useEffect, useRef } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";

interface FlightsMapProps {
  flights: FlightLog[];
  selectedFlightId: string | null;
  onSelectFlight: (flightId: string) => void;
  className?: string;
}

// Generate intermediate arc points between two coordinates for flight-like curvature
function calculateArcCoordinates(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
  numPoints = 24
): [number, number][] {
  // If coordinates are identical or invalid
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

function createAirportIcon(icao: string, isSelected: boolean) {
  return L.divIcon({
    className: "custom-airport-marker",
    html: `
      <div class="relative flex items-center justify-center cursor-pointer group">
        <div class="absolute -inset-1.5 rounded-full ${
          isSelected ? "bg-amber-400/50 animate-ping" : "bg-sky-400/25 group-hover:bg-sky-400/50"
        } transition-all"></div>
        <div class="relative flex h-5 w-5 items-center justify-center rounded-full ${
          isSelected
            ? "bg-amber-500 text-black border-2 border-white shadow-lg shadow-amber-500/50 scale-110"
            : "bg-sky-600 text-white border border-sky-300 shadow-md group-hover:scale-110"
        } transition-transform">
          <div class="h-1.5 w-1.5 rounded-full bg-white"></div>
        </div>
        <span class="absolute top-5 left-1/2 -translate-x-1/2 whitespace-nowrap rounded px-1.5 py-0.2 text-[10px] font-mono font-bold tracking-wider ${
          isSelected
            ? "bg-amber-500 text-black shadow-md border border-amber-300 scale-105"
            : "bg-black/85 text-sky-200 border border-border/80 shadow-xs"
        } transition-all">
          ${icao}
        </span>
      </div>
    `,
    iconSize: [22, 22],
    iconAnchor: [11, 11],
    popupAnchor: [0, -14],
  });
}

export function FlightsMap({
  flights,
  selectedFlightId,
  onSelectFlight,
  className = "",
}: FlightsMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<L.Map | null>(null);
  const layersGroupRef = useRef<L.FeatureGroup | null>(null);

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

      // CartoDB Dark Matter tile layer
      L.tileLayer("https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png", {
        subdomains: "abcd",
        maxZoom: 19,
      }).addTo(map);

      const layersGroup = L.featureGroup().addTo(map);
      layersGroupRef.current = layersGroup;
      mapRef.current = map;

      // Invalidate size on container resize
      const observer = new ResizeObserver(() => {
        map.invalidateSize();
      });
      observer.observe(containerRef.current);

      return () => {
        observer.disconnect();
        map.remove();
        mapRef.current = null;
        layersGroupRef.current = null;
      };
    } catch {
      // Graceful fallback for test runners / environments without Canvas/WebGL
    }
  }, []);

  // Update routes, polylines, and markers when flights or selection change
  useEffect(() => {
    const map = mapRef.current;
    const layersGroup = layersGroupRef.current;
    if (!map || !layersGroup) return;

    layersGroup.clearLayers();

    const airportMap = new Map<string, { icao: string; name: string; lat: number; lon: number }>();
    const validFlightsWithCoords = flights.filter(
      (f) =>
        f.departure_airport?.latitude &&
        f.departure_airport?.longitude &&
        f.arrival_airport?.latitude &&
        f.arrival_airport?.longitude
    );

    let selectedFlightBounds: L.LatLngBounds | null = null;

    validFlightsWithCoords.forEach((flight) => {
      const isSelected = flight.id === selectedFlightId;
      const dep = flight.departure_airport;
      const arr = flight.arrival_airport;

      if (!airportMap.has(dep.icao)) {
        airportMap.set(dep.icao, {
          icao: dep.icao,
          name: dep.name || dep.city || dep.icao,
          lat: dep.latitude,
          lon: dep.longitude,
        });
      }
      if (!airportMap.has(arr.icao)) {
        airportMap.set(arr.icao, {
          icao: arr.icao,
          name: arr.name || arr.city || arr.icao,
          lat: arr.latitude,
          lon: arr.longitude,
        });
      }

      // Draw curved flight path
      const arcPoints = calculateArcCoordinates(
        dep.latitude,
        dep.longitude,
        arr.latitude,
        arr.longitude
      );

      const polyline = L.polyline(arcPoints, {
        color: isSelected ? "#f59e0b" : "#38bdf8",
        weight: isSelected ? 4 : 2,
        opacity: isSelected ? 0.95 : 0.45,
        dashArray: isSelected ? undefined : "6, 8",
        lineCap: "round",
        lineJoin: "round",
      });

      // Hover / Click behavior on route
      polyline.on("mouseover", () => {
        if (!isSelected) {
          polyline.setStyle({ color: "#67e8f9", weight: 3.5, opacity: 0.85 });
        }
      });
      polyline.on("mouseout", () => {
        if (!isSelected) {
          polyline.setStyle({ color: "#38bdf8", weight: 2, opacity: 0.45 });
        }
      });
      polyline.on("click", () => {
        onSelectFlight(flight.id);
      });

      const popupContent = `
        <div class="text-xs p-1 text-slate-100 font-sans space-y-1">
          <div class="font-bold text-sm text-sky-400 flex items-center justify-between gap-4">
            <span>${flight.callsign}</span>
            <span class="text-xs px-1.5 py-0.5 rounded bg-sky-950 text-sky-300 font-mono">${flight.aircraft?.icao_code || "A/C"}</span>
          </div>
          <div class="text-[11px] text-slate-300 font-medium">
            ${dep.icao} ➔ ${arr.icao}
          </div>
          <div class="grid grid-cols-2 gap-x-2 gap-y-0.5 text-[10px] text-slate-400 pt-1 border-t border-slate-700/80">
            <div>Dist: <span class="text-slate-200">${Math.round(flight.distance_nm)} NM</span></div>
            <div>Time: <span class="text-slate-200">${flight.flight_time_minutes} min</span></div>
            <div>Landing: <span class="text-slate-200">${flight.landing_rate_fpm ? `${Math.round(flight.landing_rate_fpm)} fpm` : "N/A"}</span></div>
            <div>Score: <span class="text-slate-200">${flight.score || "100"}</span></div>
          </div>
        </div>
      `;
      polyline.bindPopup(popupContent, { className: "dark-custom-popup", closeButton: false });

      layersGroup.addLayer(polyline);

      if (isSelected) {
        selectedFlightBounds = L.latLngBounds([
          [dep.latitude, dep.longitude],
          [arr.latitude, arr.longitude],
        ]);
        polyline.openPopup();
      }
    });

    // Render airport markers
    airportMap.forEach((airport) => {
      const isSelectedAirport =
        Boolean(selectedFlightId) &&
        flights.some(
          (f) =>
            f.id === selectedFlightId &&
            (f.departure_airport?.icao === airport.icao || f.arrival_airport?.icao === airport.icao)
        );

      const marker = L.marker([airport.lat, airport.lon], {
        icon: createAirportIcon(airport.icao, isSelectedAirport),
        zIndexOffset: isSelectedAirport ? 1000 : 100,
      });

      marker.bindPopup(
        `
        <div class="text-xs p-1 text-slate-100 font-sans">
          <div class="font-bold text-sky-400 text-sm font-mono">${airport.icao}</div>
          <div class="text-[11px] text-slate-300">${airport.name}</div>
        </div>
        `,
        { className: "dark-custom-popup", closeButton: false }
      );

      layersGroup.addLayer(marker);
    });

    // Fit view bounds
    if (selectedFlightBounds) {
      map.fitBounds(selectedFlightBounds, { padding: [60, 60], maxZoom: 8, animate: true });
    } else if (validFlightsWithCoords.length > 0) {
      const allBounds = layersGroup.getBounds();
      if (allBounds.isValid()) {
        map.fitBounds(allBounds, { padding: [40, 40], maxZoom: 7, animate: true });
      }
    }
  }, [flights, selectedFlightId, onSelectFlight]);

  const handleResetBounds = () => {
    if (!mapRef.current || !layersGroupRef.current) return;
    const bounds = layersGroupRef.current.getBounds();
    if (bounds.isValid()) {
      mapRef.current.fitBounds(bounds, { padding: [40, 40], animate: true });
    }
  };

  return (
    <div className={`relative h-full w-full overflow-hidden rounded-xl border border-border/70 bg-card ${className}`}>
      <div ref={containerRef} className="h-full w-full bg-[#111622]" />

      {/* Map Control Bar Overlay */}
      <div className="absolute top-3 left-3 z-[400] flex items-center gap-2 rounded-lg border border-border/60 bg-background/85 px-3 py-1.5 shadow-md backdrop-blur-md text-xs">
        <span className="flex h-2 w-2 rounded-full bg-sky-400 animate-pulse" />
        <span className="font-medium text-foreground">
          {flights.length} {flights.length === 1 ? "Voo mapeado" : "Voos mapeados"}
        </span>
        <button
          type="button"
          onClick={handleResetBounds}
          className="ml-2 rounded px-2 py-0.5 text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground transition-colors border border-border/50"
        >
          Centralizar
        </button>
      </div>

      {flights.length === 0 && (
        <div className="absolute inset-0 z-[400] flex flex-col items-center justify-center bg-background/50 backdrop-blur-xs text-center p-4">
          <p className="text-sm font-medium text-muted-foreground">
            Nenhum voo recente com coordenadas disponíveis para exibição no mapa.
          </p>
        </div>
      )}
    </div>
  );
}
