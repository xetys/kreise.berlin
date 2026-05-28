'use client';

import {use, useCallback, useEffect, useRef, useState} from 'react';
import {usePathname} from 'next/navigation';
import Link from 'next/link';
import {api, APIError} from '@/lib/api';

interface PageProps {
  params: Promise<{locale: string; id: string}>;
}

// QR token in the ticket email is a base64url string with two dots. We feed
// it to the backend verbatim; the camera library just hands us whatever's
// inside the QR. Most ticket QRs encode the full URL of the ticket page,
// but the scanner also tolerates the bare token format.
function extractToken(raw: string): string {
  const trimmed = raw.trim();
  // If a URL was scanned, the last path segment is the token. Our ticket
  // URL is `/{locale}/tickets/{token}` so taking the last segment works.
  try {
    const u = new URL(trimmed);
    const parts = u.pathname.split('/').filter(Boolean);
    return parts.length > 0 ? parts[parts.length - 1] : trimmed;
  } catch {
    return trimmed;
  }
}

interface CheckInResp {
  outcome: 'ok' | 'already_checked_in' | 'needs_amount';
  ticket_id: string;
  participant_name: string;
  participant_email?: string;
  booking_reference: string;
  checked_in_at?: string;
  prior_checked_in_at?: string;
  booking_payment_method?: string;
  event_pricing_mode?: string;
  expected_amount_minor?: number;
  paid_amount_minor?: number;
  currency?: string;
}

interface StatusResp {
  expected: number;
  checked_in: number;
  recent: Array<{
    id: string;
    participant_name: string;
    booking_reference: string;
    checked_in_at?: string;
    paid_amount_minor?: number;
  }>;
}

interface ManualTicket {
  id: string;
  participant_name: string;
  participant_email?: string;
  booking_reference: string;
  checked_in_at?: string;
}

type FeedbackKind = 'ok' | 'already' | 'error';
interface Feedback {
  kind: FeedbackKind;
  title: string;
  detail?: string;
}

const SCAN_REGION_ID = 'kreise-scan-region';

export default function CheckInPage({params}: PageProps) {
  const {id} = use(params);
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';

  const [status, setStatus] = useState<StatusResp | null>(null);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [cameraOn, setCameraOn] = useState(false);
  const [cameraError, setCameraError] = useState<string | null>(null);

  // Manual entry state
  const [manualRef, setManualRef] = useState('');
  const [manualMatches, setManualMatches] = useState<ManualTicket[] | null>(null);
  const [manualBusy, setManualBusy] = useState(false);

  // Refs for scanner instance + de-bouncing
  const scannerRef = useRef<{stop: () => Promise<void>; clear: () => void} | null>(null);
  // Don't re-submit the same code while staff lingers it in the frame.
  const lastTokenRef = useRef<{token: string; at: number} | null>(null);
  const busyRef = useRef(false);

  // Amount-modal state: when the backend responds with `needs_amount` we
  // pop this modal so the staffer types / confirms the cash collected.
  // Submitting it re-fires the same endpoint (POST scan or scan/manual)
  // with paid_amount_minor included.
  const [amountPrompt, setAmountPrompt] = useState<{
    via: 'scan' | 'manual';
    token?: string;
    ticketId?: string;
    participantName: string;
    bookingReference: string;
    expectedMinor: number;
    currency: string;
    pricingMode?: string;
    paymentMethod?: string;
    amountInput: string; // controlled text — user types here
  } | null>(null);

  const refreshStatus = useCallback(async () => {
    try {
      const r = await api<StatusResp>(`/api/admin/events/${id}/check-ins`);
      setStatus(r);
    } catch (err) {
      // Non-fatal; the scanner still works without the status strip.
      console.warn('status refresh failed', err);
    }
  }, [id]);

  useEffect(() => {
    refreshStatus();
    const t = setInterval(refreshStatus, 5000);
    return () => clearInterval(t);
  }, [refreshStatus]);

  // applyCheckInResponse handles the universal response shape coming back
  // from /scan and /scan/manual. Three branches:
  //   - "needs_amount" → open the amount modal (no audio cue yet — the
  //     check-in hasn't happened)
  //   - "already_checked_in" → amber warn cue + banner
  //   - "ok" → green ok cue + banner showing collected amount when present
  function applyCheckInResponse(r: CheckInResp, via: 'scan' | 'manual', tokenOrTicket?: string) {
    if (r.outcome === 'needs_amount') {
      // For matrix at-door we PRE-FILL with the expected price (staffer
      // usually just presses Confirm). For donation we leave the input
      // empty so the staffer types the actual amount; pre-filling the
      // suggested would anchor too strongly.
      const isDonation = r.event_pricing_mode === 'donation' || r.booking_payment_method === 'donation';
      const defaultMinor = isDonation ? null : r.expected_amount_minor ?? null;
      setAmountPrompt({
        via,
        token: via === 'scan' ? tokenOrTicket : undefined,
        ticketId: r.ticket_id,
        participantName: r.participant_name,
        bookingReference: r.booking_reference,
        expectedMinor: r.expected_amount_minor ?? 0,
        currency: r.currency ?? 'EUR',
        pricingMode: r.event_pricing_mode,
        paymentMethod: r.booking_payment_method,
        amountInput: defaultMinor !== null ? minorToInput(defaultMinor) : '',
      });
      return;
    }
    playCue(r.outcome === 'already_checked_in' ? 'warn' : 'ok');
    const amount = formatAmountOrNull(r.paid_amount_minor, r.currency);
    setFeedback({
      kind: r.outcome === 'already_checked_in' ? 'already' : 'ok',
      title: r.participant_name,
      detail:
        r.outcome === 'already_checked_in' && r.prior_checked_in_at
          ? `Bereits um ${fmtTime(r.prior_checked_in_at)}${amount ? ' · ' + amount : ''}`
          : `${r.booking_reference} · ${r.checked_in_at ? fmtTime(r.checked_in_at) : ''}${amount ? ' · ' + amount : ''}`,
    });
    refreshStatus();
  }

  const submitScan = useCallback(
    async (raw: string) => {
      if (busyRef.current) return;
      const token = extractToken(raw);
      const now = Date.now();
      if (lastTokenRef.current && lastTokenRef.current.token === token && now - lastTokenRef.current.at < 3000) {
        return; // de-dupe rapid duplicate frames
      }
      lastTokenRef.current = {token, at: now};
      busyRef.current = true;
      try {
        const r = await api<CheckInResp>(`/api/admin/events/${id}/scan`, {
          method: 'POST',
          body: {token},
        });
        applyCheckInResponse(r, 'scan', token);
      } catch (err) {
        playCue('error');
        if (err instanceof APIError) {
          setFeedback({kind: 'error', title: scanErrorTitle(err.code), detail: err.developerMessage});
        } else {
          setFeedback({kind: 'error', title: 'Fehler', detail: String(err)});
        }
      } finally {
        setTimeout(() => {
          busyRef.current = false;
        }, 1500);
      }
    },
    // applyCheckInResponse closes over setAmountPrompt etc.; refreshStatus is the
    // only stable dep we need.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [id, refreshStatus]
  );

  // Phase 2: the staffer submitted the amount modal. Re-fires the same
  // endpoint with paid_amount_minor (or confirm_zero) populated.
  async function submitAmountConfirm() {
    if (!amountPrompt) return;
    const minor = parseInputToMinor(amountPrompt.amountInput);
    if (minor === null) {
      setFeedback({kind: 'error', title: 'Ungültiger Betrag', detail: 'Bitte als Zahl eingeben (z. B. 12,50)'});
      return;
    }
    const body: Record<string, unknown> = {
      paid_amount_minor: minor,
      confirm_zero: minor === 0,
    };
    if (amountPrompt.via === 'scan') {
      body.token = amountPrompt.token;
    } else {
      body.ticket_id = amountPrompt.ticketId;
    }
    const url =
      amountPrompt.via === 'scan'
        ? `/api/admin/events/${id}/scan`
        : `/api/admin/events/${id}/scan/manual`;
    try {
      const r = await api<CheckInResp>(url, {method: 'POST', body});
      setAmountPrompt(null);
      applyCheckInResponse(r, amountPrompt.via);
    } catch (err) {
      playCue('error');
      setFeedback({
        kind: 'error',
        title: 'Check-in fehlgeschlagen',
        detail: err instanceof APIError ? err.developerMessage : String(err),
      });
    }
  }

  async function undoCheckIn(ticketId: string) {
    if (!confirm('Check-in rückgängig machen?')) return;
    try {
      await api(`/api/admin/events/${id}/scan/undo`, {
        method: 'POST',
        body: {ticket_id: ticketId},
      });
      refreshStatus();
      setFeedback({kind: 'already', title: 'Rückgängig', detail: 'Check-in entfernt.'});
    } catch (err) {
      setFeedback({
        kind: 'error',
        title: 'Rückgängig fehlgeschlagen',
        detail: err instanceof APIError ? err.developerMessage : String(err),
      });
    }
  }

  // Initialise the camera scanner once when the user opts in.
  useEffect(() => {
    if (!cameraOn) return;
    let cancelled = false;
    setCameraError(null);
    (async () => {
      try {
        // Lazy-load the library (browser-only) — keeps SSR clean.
        const {Html5Qrcode} = await import('html5-qrcode');
        if (cancelled) return;
        const scanner = new Html5Qrcode(SCAN_REGION_ID, false);
        scannerRef.current = {
          stop: () => scanner.stop(),
          clear: () => scanner.clear(),
        };
        await scanner.start(
          {facingMode: 'environment'},
          {fps: 10, qrbox: {width: 240, height: 240}},
          (decoded) => submitScan(decoded),
          () => {/* per-frame failures are noisy; ignore */}
        );
      } catch (err) {
        if (!cancelled) {
          setCameraError(
            err instanceof Error
              ? err.message
              : 'Kamera-Zugriff verweigert. Erlaube den Zugriff in den Browser-Einstellungen.'
          );
          setCameraOn(false);
        }
      }
    })();
    return () => {
      cancelled = true;
      const sc = scannerRef.current;
      scannerRef.current = null;
      if (sc) {
        sc.stop().catch(() => {}).finally(() => sc.clear());
      }
    };
  }, [cameraOn, submitScan]);

  // Debounced auto-search: typing 2+ characters fires the lookup 300ms after
  // the last keystroke. Pressing Enter submits immediately via the form.
  useEffect(() => {
    const q = manualRef.trim();
    if (q.length < 2) {
      setManualMatches(null);
      return;
    }
    const handle = setTimeout(() => {
      runManualLookup(q).catch(() => {});
    }, 300);
    return () => clearTimeout(handle);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [manualRef]);

  async function runManualLookup(query: string) {
    setManualBusy(true);
    try {
      const r = await api<{tickets: ManualTicket[]}>(`/api/admin/events/${id}/scan/manual`, {
        method: 'POST',
        body: {reference: query},
      });
      setManualMatches(r.tickets);
    } catch (err) {
      // Don't show a banner for "too short" / debounce-canceled lookups;
      // only surface real failures.
      if (err instanceof APIError && err.code !== 'query_too_short') {
        setFeedback({
          kind: 'error',
          title: 'Suche fehlgeschlagen',
          detail: err.developerMessage,
        });
      }
      setManualMatches([]);
    } finally {
      setManualBusy(false);
    }
  }

  async function manualLookup(e: React.FormEvent) {
    e.preventDefault();
    const q = manualRef.trim();
    if (q.length < 2) return;
    await runManualLookup(q);
  }

  async function manualCheckIn(ticketId: string) {
    setManualBusy(true);
    try {
      const r = await api<CheckInResp>(`/api/admin/events/${id}/scan/manual`, {
        method: 'POST',
        body: {ticket_id: ticketId},
      });
      // Same response shape as the camera scan — needs_amount opens the
      // modal, otherwise we show the feedback banner.
      if (r.outcome !== 'needs_amount') {
        setManualMatches(null);
        setManualRef('');
      }
      applyCheckInResponse(r, 'manual', ticketId);
    } catch (err) {
      setFeedback({
        kind: 'error',
        title: 'Check-in fehlgeschlagen',
        detail: err instanceof APIError ? err.developerMessage : String(err),
      });
    } finally {
      setManualBusy(false);
    }
  }

  return (
    <main className="flex-1 max-w-3xl mx-auto w-full px-4 py-6 flex flex-col gap-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <Link
            href={`/${localePrefix}/admin/events/${id}`}
            className="text-xs text-neutral-500 hover:underline"
          >
            ← Veranstaltung
          </Link>
          <h1 className="text-xl sm:text-2xl font-light tracking-wide mt-1">Einlass</h1>
        </div>
        {status && (
          <div className="rounded-md border border-neutral-200 dark:border-neutral-800 px-3 py-2 text-right">
            <div className="text-[10px] uppercase tracking-wider opacity-60">Eingecheckt</div>
            <div className="text-xl font-medium tabular-nums">
              {status.checked_in}
              <span className="opacity-50 text-base"> / {status.expected}</span>
            </div>
          </div>
        )}
      </header>

      {/* Scanner */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3 flex flex-col gap-3">
        {!cameraOn && (
          <button
            onClick={() => setCameraOn(true)}
            className="rounded-md bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 px-4 py-3 text-base font-medium"
          >
            Kamera starten
          </button>
        )}
        {cameraError && (
          <p className="text-sm text-red-700 dark:text-red-300">{cameraError}</p>
        )}
        {cameraOn && (
          <>
            <div className="relative w-full aspect-square max-h-[60vh] mx-auto bg-black rounded-md overflow-hidden">
              <div id={SCAN_REGION_ID} className="w-full h-full" />
              {feedback && (
                <button
                  onClick={() => setFeedback(null)}
                  className={`absolute inset-x-0 bottom-0 px-4 py-4 text-left ${
                    feedback.kind === 'ok'
                      ? 'bg-green-600 text-white'
                      : feedback.kind === 'already'
                        ? 'bg-amber-500 text-amber-950'
                        : 'bg-red-600 text-white'
                  }`}
                >
                  <div className="text-2xl font-medium">
                    {feedback.kind === 'ok' ? '✓ ' : feedback.kind === 'already' ? '⚠ ' : '✗ '}
                    {feedback.title}
                  </div>
                  {feedback.detail && <div className="text-sm opacity-90 mt-0.5">{feedback.detail}</div>}
                  <div className="text-[10px] opacity-70 mt-1">Tippen zum Ausblenden</div>
                </button>
              )}
            </div>
            <button
              onClick={() => setCameraOn(false)}
              className="text-xs opacity-60 hover:opacity-100 self-end"
            >
              Kamera stoppen
            </button>
          </>
        )}
      </section>

      {/* Manual entry — wider than just-by-reference now: name, email, or
          booking reference all work. Auto-searches as the staffer types,
          debounced 300ms, so they can scrub the list without re-tapping. */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3 flex flex-col gap-2">
        <h2 className="text-sm font-medium">Person suchen</h2>
        <p className="text-xs opacity-60">
          Name, E-Mail oder Buchungs-Referenz — auch Teilstrings funktionieren. Tippe auf eine Person
          in der Liste, um sie einzuchecken.
        </p>
        <form onSubmit={manualLookup} className="flex gap-2">
          <input
            type="search"
            value={manualRef}
            onChange={(e) => setManualRef(e.target.value)}
            placeholder="z. B. Anna, mail@beispiel.de, A1B2C3D4"
            className="flex-1 rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-base"
            autoComplete="off"
          />
          <button
            type="submit"
            disabled={manualBusy || manualRef.trim().length < 2}
            className="rounded-md border border-neutral-300 dark:border-neutral-700 px-3 py-2 text-sm disabled:opacity-50"
          >
            Suchen
          </button>
        </form>
        {manualMatches && manualMatches.length > 0 && (
          <ul className="flex flex-col divide-y divide-neutral-200 dark:divide-neutral-800">
            {manualMatches.map((t) => (
              <li key={t.id} className="py-2 flex items-center justify-between gap-3">
                <div className="text-sm">
                  <div>{t.participant_name}</div>
                  <div className="text-xs opacity-60">
                    {t.booking_reference}
                    {t.checked_in_at && (
                      <>
                        {' '}
                        · <span className="text-amber-700 dark:text-amber-300">
                          bereits {fmtTime(t.checked_in_at)}
                        </span>
                      </>
                    )}
                  </div>
                </div>
                <button
                  onClick={() => manualCheckIn(t.id)}
                  disabled={manualBusy}
                  className="rounded-md bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 px-3 py-1.5 text-sm disabled:opacity-50"
                >
                  {t.checked_in_at ? 'Erneut' : 'Einchecken'}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* Recent activity */}
      {status && status.recent.length > 0 && (
        <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-3 flex flex-col gap-1">
          <h2 className="text-sm font-medium">Zuletzt eingecheckt</h2>
          <ul className="flex flex-col divide-y divide-neutral-200 dark:divide-neutral-800 text-sm">
            {status.recent.map((r) => (
              <li key={r.id} className="py-1.5 flex items-center justify-between gap-3">
                <div className="flex flex-col">
                  <span>{r.participant_name}</span>
                  <span className="text-xs opacity-60">
                    {r.checked_in_at ? fmtTime(r.checked_in_at) : ''} · {r.booking_reference}
                    {r.paid_amount_minor !== undefined && r.paid_amount_minor !== null && (
                      <> · <span className="font-medium">{formatMinor(r.paid_amount_minor, 'EUR')}</span></>
                    )}
                  </span>
                </div>
                <button
                  onClick={() => undoCheckIn(r.id)}
                  className="text-xs text-red-700 dark:text-red-300 hover:underline"
                >
                  rückgängig
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Amount-input modal (at_door + donation). Rendered last so it sits
          on top of everything; backdrop click cancels. The header shows
          the participant + booking reference so the staffer doesn't lose
          context if the modal pops mid-stream. */}
      {amountPrompt && (
        <div
          className="fixed inset-0 z-50 bg-black/60 flex items-end sm:items-center justify-center p-3"
          onClick={() => setAmountPrompt(null)}
        >
          <div
            className="w-full max-w-md bg-white dark:bg-neutral-900 rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 flex flex-col gap-3"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex flex-col gap-0.5">
              <h2 className="text-base font-medium">{amountPrompt.participantName}</h2>
              <p className="text-xs opacity-60">
                {amountPrompt.bookingReference} ·{' '}
                {amountPrompt.pricingMode === 'donation'
                  ? 'Spende — wieviel hat die Person bezahlt?'
                  : `Empfohlener Preis: ${formatMinor(amountPrompt.expectedMinor, amountPrompt.currency)}`}
              </p>
            </div>
            <label className="flex flex-col gap-1">
              <span className="text-xs uppercase tracking-wide opacity-70">
                Eingenommen ({amountPrompt.currency})
              </span>
              <input
                type="number"
                step="0.01"
                min={0}
                inputMode="decimal"
                autoFocus
                value={amountPrompt.amountInput}
                onChange={(e) => setAmountPrompt({...amountPrompt, amountInput: e.target.value})}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') submitAmountConfirm();
                }}
                className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-3 text-2xl font-mono"
                placeholder={amountPrompt.pricingMode === 'donation' ? '0,00' : minorToInput(amountPrompt.expectedMinor)}
              />
              <span className="text-[10px] opacity-50">
                Leer / 0 bedeutet „nichts kassiert" (z. B. Gast eingeladen). Tippe Enter zum
                Bestätigen.
              </span>
            </label>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setAmountPrompt(null)}
                className="rounded-md border border-neutral-300 dark:border-neutral-700 px-3 py-2 text-sm"
              >
                Abbrechen
              </button>
              <button
                onClick={submitAmountConfirm}
                className="rounded-md bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm font-medium"
              >
                Check-in
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

// minorToInput converts a backend minor-unit amount (12345) to the form
// input string ("123,45"). Comma-as-decimal because the input is rendered
// in German locale-leaning context.
function minorToInput(minor: number): string {
  if (minor === 0) return '0';
  const whole = Math.floor(minor / 100);
  const cents = Math.abs(minor % 100);
  return `${whole},${String(cents).padStart(2, '0')}`;
}

// parseInputToMinor turns "12,50" / "12.50" / "12" into 1250. Returns null
// for empty / non-numeric input so the caller can prompt for a retry. Empty
// string is treated as 0 (the staffer explicitly empties to mark "comped").
function parseInputToMinor(s: string): number | null {
  const t = s.trim().replace(',', '.');
  if (t === '') return 0;
  const n = Number(t);
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.round(n * 100);
}

function formatMinor(minor: number, currency: string): string {
  const whole = Math.floor(minor / 100);
  const cents = Math.abs(minor % 100);
  return `${whole},${String(cents).padStart(2, '0')} ${currency}`;
}

function formatAmountOrNull(minor: number | undefined | null, currency: string | undefined): string | null {
  if (minor === undefined || minor === null) return null;
  return formatMinor(minor, currency ?? 'EUR');
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString('de-DE', {hour: '2-digit', minute: '2-digit'});
}

function scanErrorTitle(code: string): string {
  switch (code) {
    case 'invalid_token':
      return 'Ungültiger QR-Code';
    case 'not_found':
      return 'Ticket nicht gefunden';
    case 'wrong_event':
      return 'Ticket für andere Veranstaltung';
    case 'stale_token':
      return 'QR veraltet (Ticket übertragen)';
    case 'canceled':
      return 'Ticket storniert';
    case 'unpaid':
      return 'Ticket noch nicht bezahlt';
    case 'forbidden':
      return 'Keine Berechtigung';
    default:
      return `Fehler: ${code}`;
  }
}

// Plays a short tone via WebAudio so door staff don't need to look at the
// screen for confirmation. Different timbres for ok / warn / error keeps
// scanning fast in a noisy line.
function playCue(kind: 'ok' | 'warn' | 'error') {
  try {
    const Ctor =
      typeof window !== 'undefined'
        ? (window as unknown as {AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext}).AudioContext ??
          (window as unknown as {webkitAudioContext?: typeof AudioContext}).webkitAudioContext
        : undefined;
    if (!Ctor) return;
    const ctx = new Ctor();
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = 'sine';
    o.frequency.value = kind === 'ok' ? 880 : kind === 'warn' ? 660 : 220;
    o.connect(g);
    g.connect(ctx.destination);
    g.gain.setValueAtTime(0.15, ctx.currentTime);
    g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.18);
    o.start();
    o.stop(ctx.currentTime + 0.18);
    o.onended = () => ctx.close();
  } catch {
    // no-op
  }
  // Best-effort haptic too — Android Chrome supports it.
  if (typeof navigator !== 'undefined' && 'vibrate' in navigator) {
    try {
      (navigator as Navigator & {vibrate?: (pattern: number | number[]) => boolean}).vibrate?.(
        kind === 'ok' ? 30 : kind === 'warn' ? [20, 60, 20] : [60, 60, 60]
      );
    } catch {
      // no-op
    }
  }
}
