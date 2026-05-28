'use client';

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';

interface Coords {
  lat: number;
  lon: number;
  bbox?: [number, number, number, number]; // [minLat, maxLat, minLon, maxLon]
}

const STORAGE_KEY = 'tg.geocode.v1';

function loadCache(): Record<string, Coords | null> {
  if (typeof window === 'undefined') return {};
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}');
  } catch {
    return {};
  }
}

function saveCache(cache: Record<string, Coords | null>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cache));
  } catch {
    // localStorage full / disabled — no-op, the cache is best-effort.
  }
}

export function EventLocationMap({location}: {location: string}) {
  const t = useTranslations('OSM');
  // undefined = loading, null = no result, Coords = resolved.
  const [coords, setCoords] = useState<Coords | null | undefined>(undefined);

  useEffect(() => {
    const cache = loadCache();
    if (location in cache) {
      setCoords(cache[location]);
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        // Nominatim usage policy: ≤1 req/sec, cache results, no heavy automated use.
        // We cache hits in localStorage so the same browser only hits the API once
        // per unique location string. Accept-Language=de gets German place names back.
        const url =
          `https://nominatim.openstreetmap.org/search?q=${encodeURIComponent(location)}` +
          `&format=json&limit=1`;
        const res = await fetch(url, {headers: {'Accept-Language': 'de'}});
        if (!res.ok) throw new Error('geocode_http_' + res.status);
        const data = await res.json();
        if (cancelled) return;

        if (Array.isArray(data) && data[0]?.lat && data[0]?.lon) {
          const item = data[0];
          let bbox: Coords['bbox'];
          if (Array.isArray(item.boundingbox) && item.boundingbox.length === 4) {
            bbox = [
              parseFloat(item.boundingbox[0]),
              parseFloat(item.boundingbox[1]),
              parseFloat(item.boundingbox[2]),
              parseFloat(item.boundingbox[3]),
            ];
          }
          const result: Coords = {lat: parseFloat(item.lat), lon: parseFloat(item.lon), bbox};
          cache[location] = result;
          saveCache(cache);
          setCoords(result);
        } else {
          cache[location] = null;
          saveCache(cache);
          setCoords(null);
        }
      } catch {
        if (!cancelled) setCoords(null);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [location]);

  const searchUrl = `https://www.openstreetmap.org/search?query=${encodeURIComponent(location)}`;

  if (coords === undefined) {
    return (
      <div className="rounded-xl bg-black/5 dark:bg-white/5 h-48 sm:h-56 flex items-center justify-center text-xs opacity-60">
        {t('loading')}
      </div>
    );
  }

  if (coords === null) {
    return (
      <div className="text-center text-sm">
        <a
          href={searchUrl}
          target="_blank"
          rel="noreferrer"
          className="underline opacity-80 hover:opacity-100"
        >
          {t('searchFallback')}
        </a>
      </div>
    );
  }

  // Always render a fixed-size box around the marker, ignoring Nominatim's
  // own bbox. Address-level results return a bbox so tight (~0.0001°) that
  // the map shows only the building — the user wants neighborhood context to
  // recognize "ah, that's near X". 0.015° on each axis gives roughly a
  // 1.5×1.5 km window, which fits "the U-Bahn station and a couple of cross
  // streets" at Berlin scale.
  const PAD = 0.015;
  const minLat = coords.lat - PAD;
  const maxLat = coords.lat + PAD;
  const minLon = coords.lon - PAD;
  const maxLon = coords.lon + PAD;
  const bboxStr = `${minLon},${minLat},${maxLon},${maxLat}`;
  const embedUrl =
    `https://www.openstreetmap.org/export/embed.html?bbox=${encodeURIComponent(bboxStr)}` +
    `&layer=mapnik&marker=${coords.lat},${coords.lon}`;
  const linkUrl =
    `https://www.openstreetmap.org/?mlat=${coords.lat}&mlon=${coords.lon}` +
    `#map=16/${coords.lat}/${coords.lon}`;

  return (
    <div className="flex flex-col gap-2">
      <div className="rounded-xl overflow-hidden border border-black/10 shadow-sm bg-white">
        <iframe
          title={t('iframeTitle')}
          src={embedUrl}
          width="100%"
          height="240"
          loading="lazy"
          referrerPolicy="no-referrer-when-downgrade"
          style={{border: 0, display: 'block'}}
        />
      </div>
      <div className="text-center text-[10px] uppercase tracking-[0.22em] opacity-60">
        <a href={linkUrl} target="_blank" rel="noreferrer" className="hover:underline">
          {t('largerMap')}
        </a>
      </div>
    </div>
  );
}
