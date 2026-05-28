'use client';

import {useCallback, useEffect, useMemo, useState} from 'react';
import {api, APIError} from '@/lib/api';

// ============================================================================
// Types (mirror backend pricingSnapshot DTO)
// ============================================================================

interface Phase {
  id: string;
  name: string;
  starts_at: string;
  ends_at: string;
  ordering: number;
}

interface Category {
  id: string;
  name: string;
  ordering: number;
}

interface Duration {
  id: string;
  name: string;
  ordering: number;
}

interface Price {
  id: string;
  phase_id: string;
  category_id: string;
  duration_id: string | null;
  amount_minor: number;
}

interface DonationConfig {
  suggested_minor: number;
  min_minor: number;
}

interface Coupon {
  id: string;
  code: string;
  type: 'fixed_reduce' | 'percental_reduce' | 'guestlist';
  value_minor: number | null;
  value_percent: number | null;
  max_uses: number | null;
  valid_from: string | null;
  valid_to: string | null;
  single_use_per_email: boolean;
}

interface Snapshot {
  pricing_mode: 'matrix' | 'donation';
  currency: string;
  donation_config: DonationConfig | null;
  phases: Phase[];
  categories: Category[];
  durations: Duration[];
  prices: Price[];
  coupons: Coupon[];
}

// ============================================================================
// Conversion helpers (cents ↔ display)
// ============================================================================

function centsToDisplay(c: number): string {
  return (c / 100).toFixed(2);
}

function displayToCents(s: string): number | null {
  const trimmed = s.trim();
  if (!trimmed) return null;
  const n = Number(trimmed.replace(',', '.'));
  if (!Number.isFinite(n) || n < 0) return null;
  return Math.round(n * 100);
}

// ============================================================================
// Top-level editor
// ============================================================================

export function PricingEditor({eventId, mode, currency}: {eventId: string; mode: 'matrix' | 'donation'; currency: string}) {
  const [snap, setSnap] = useState<Snapshot | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await api<Snapshot>(`/api/admin/events/${eventId}/pricing`);
      setSnap(s);
      setError(null);
    } catch (e) {
      setError(e instanceof APIError ? e.code : String(e));
    }
  }, [eventId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (error) return <p className="text-sm text-red-600">{error}</p>;
  if (!snap) return <p className="text-xs text-neutral-500">Lädt Preise…</p>;

  return (
    <section className="flex flex-col gap-6 border-t border-neutral-200 dark:border-neutral-800 pt-4">
      <h2 className="text-sm font-medium">Preise</h2>
      <p className="text-xs text-neutral-500">
        Modus: <strong>{mode === 'donation' ? 'Spende' : 'Matrix'}</strong> · Währung: <strong>{currency}</strong>
      </p>

      {mode === 'donation' ? (
        <DonationBlock eventId={eventId} cfg={snap.donation_config} onSaved={refresh} />
      ) : (
        <>
          <PhasesList eventId={eventId} phases={snap.phases} onChange={refresh} />
          <NamedList
            title="Kategorien"
            entityPath="categories"
            entityIdKey="catId"
            items={snap.categories}
            eventId={eventId}
            onChange={refresh}
          />
          <NamedList
            title="Dauer (optional)"
            entityPath="durations"
            entityIdKey="durId"
            items={snap.durations}
            eventId={eventId}
            onChange={refresh}
            hint="Lass leer, wenn dein Event keine Dauer-Dimension braucht (z. B. nur ein Eintritt)."
          />
          <PriceGrid eventId={eventId} snap={snap} onChange={refresh} />
        </>
      )}

      <CouponsBlock eventId={eventId} coupons={snap.coupons} onChange={refresh} />
    </section>
  );
}

// ============================================================================
// Donation block
// ============================================================================

function DonationBlock({eventId, cfg, onSaved}: {eventId: string; cfg: DonationConfig | null; onSaved: () => Promise<void>}) {
  const [suggested, setSuggested] = useState(cfg ? centsToDisplay(cfg.suggested_minor) : '0.00');
  const [min, setMin] = useState(cfg ? centsToDisplay(cfg.min_minor) : '0.00');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    if (cfg) {
      setSuggested(centsToDisplay(cfg.suggested_minor));
      setMin(centsToDisplay(cfg.min_minor));
    }
  }, [cfg]);

  async function save() {
    setBusy(true);
    setErr(null);
    setSaved(false);
    try {
      const sCents = displayToCents(suggested);
      const mCents = displayToCents(min);
      if (sCents === null || mCents === null) {
        setErr('invalid_amount');
        return;
      }
      await api(`/api/admin/events/${eventId}/pricing/donation`, {
        method: 'PUT',
        body: {suggested_minor: sCents, min_minor: mCents},
      });
      await onSaved();
      setSaved(true);
    } catch (e) {
      setErr(e instanceof APIError ? e.code : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs font-medium text-neutral-500 uppercase tracking-wide">Spendenkonfiguration</h3>
      <div className="grid grid-cols-2 gap-3">
        <label className="flex flex-col gap-1 text-sm">
          Vorschlag
          <input
            type="text"
            inputMode="decimal"
            value={suggested}
            onChange={(e) => setSuggested(e.target.value)}
            className={inputCls}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          Minimum
          <input
            type="text"
            inputMode="decimal"
            value={min}
            onChange={(e) => setMin(e.target.value)}
            className={inputCls}
          />
        </label>
      </div>
      <div className="flex items-center gap-3">
        <button onClick={save} disabled={busy} className={btnPrimary}>
          {busy ? 'Speichern…' : 'Speichern'}
        </button>
        {saved && <span className="text-xs text-emerald-700">Gespeichert.</span>}
        {err && <span className="text-xs text-red-600">{err}</span>}
      </div>
    </div>
  );
}

// ============================================================================
// Phases list
// ============================================================================

function PhasesList({eventId, phases, onChange}: {eventId: string; phases: Phase[]; onChange: () => Promise<void>}) {
  const [name, setName] = useState('');
  const [startsAt, setStartsAt] = useState('');
  const [endsAt, setEndsAt] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function add() {
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/admin/events/${eventId}/pricing/phases`, {
        method: 'POST',
        body: {
          name,
          starts_at: new Date(startsAt).toISOString(),
          ends_at: new Date(endsAt).toISOString(),
          ordering: phases.length,
        },
      });
      setName('');
      setStartsAt('');
      setEndsAt('');
      await onChange();
    } catch (e) {
      setErr(e instanceof APIError ? e.code : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    if (!confirm('Phase löschen? Alle damit verbundenen Preise werden mit gelöscht.')) return;
    await api(`/api/admin/events/${eventId}/pricing/phases/${id}`, {method: 'DELETE'});
    await onChange();
  }

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs font-medium text-neutral-500 uppercase tracking-wide">Verkaufsphasen</h3>
      {phases.length === 0 && <p className="text-xs text-neutral-500">Noch keine Phasen.</p>}
      {phases.length > 0 && (
        <ul className="text-sm flex flex-col gap-1">
          {phases.map((p) => (
            <li
              key={p.id}
              className="flex items-center justify-between border-b border-neutral-100 dark:border-neutral-900 py-1"
            >
              <span>
                <strong>{p.name}</strong>{' '}
                <span className="text-xs text-neutral-500">
                  {new Date(p.starts_at).toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'})} →{' '}
                  {new Date(p.ends_at).toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'})}
                </span>
              </span>
              <button onClick={() => remove(p.id)} className="text-xs text-red-600 hover:underline">
                löschen
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex flex-wrap gap-2 text-sm items-end">
        <input
          type="text"
          placeholder="Name (z. B. early-bird)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className={inputCls + ' flex-1 min-w-[10rem]'}
        />
        <input
          type="datetime-local"
          value={startsAt}
          onChange={(e) => setStartsAt(e.target.value)}
          className={inputCls}
        />
        <input
          type="datetime-local"
          value={endsAt}
          onChange={(e) => setEndsAt(e.target.value)}
          className={inputCls}
        />
        <button onClick={add} disabled={busy || !name || !startsAt || !endsAt} className={btnSmall}>
          Hinzufügen
        </button>
      </div>
      {err && <p className="text-xs text-red-600">{err}</p>}
    </div>
  );
}

// ============================================================================
// Categories / Durations (same shape, generic component)
// ============================================================================

function NamedList({
  title,
  entityPath,
  entityIdKey: _entityIdKey,
  items,
  eventId,
  onChange,
  hint,
}: {
  title: string;
  entityPath: 'categories' | 'durations';
  entityIdKey: string;
  items: {id: string; name: string; ordering: number}[];
  eventId: string;
  onChange: () => Promise<void>;
  hint?: string;
}) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function add() {
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/admin/events/${eventId}/pricing/${entityPath}`, {
        method: 'POST',
        body: {name, ordering: items.length},
      });
      setName('');
      await onChange();
    } catch (e) {
      setErr(e instanceof APIError ? e.code : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    if (!confirm(`Eintrag "${id.slice(0, 6)}" löschen?`)) return;
    await api(`/api/admin/events/${eventId}/pricing/${entityPath}/${id}`, {method: 'DELETE'});
    await onChange();
  }

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs font-medium text-neutral-500 uppercase tracking-wide">{title}</h3>
      {hint && <p className="text-xs text-neutral-500">{hint}</p>}
      {items.length === 0 && <p className="text-xs text-neutral-500">Noch keine Einträge.</p>}
      {items.length > 0 && (
        <ul className="text-sm flex flex-wrap gap-2">
          {items.map((it) => (
            <li
              key={it.id}
              className="flex items-center gap-2 border border-neutral-200 dark:border-neutral-800 rounded px-2 py-1"
            >
              <span>{it.name}</span>
              <button onClick={() => remove(it.id)} className="text-xs text-red-600 hover:underline">
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2 text-sm">
        <input
          type="text"
          placeholder="Name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className={inputCls + ' flex-1'}
        />
        <button onClick={add} disabled={busy || !name} className={btnSmall}>
          Hinzufügen
        </button>
      </div>
      {err && <p className="text-xs text-red-600">{err}</p>}
    </div>
  );
}

// ============================================================================
// Price grid (the matrix editor itself)
// ============================================================================

interface GridRowKey {
  catId: string;
  durId: string | null;
}

function PriceGrid({eventId, snap, onChange}: {eventId: string; snap: Snapshot; onChange: () => Promise<void>}) {
  // Build the price lookup: key = "phase|cat|dur" → Price.
  const lookup = useMemo(() => {
    const m = new Map<string, Price>();
    for (const p of snap.prices) {
      m.set(priceKey(p.phase_id, p.category_id, p.duration_id), p);
    }
    return m;
  }, [snap.prices]);

  // Rows: every (category, duration?) combination. If durations is empty,
  // there's one row per category.
  const rows = useMemo<GridRowKey[]>(() => {
    const out: GridRowKey[] = [];
    if (snap.durations.length === 0) {
      for (const c of snap.categories) {
        out.push({catId: c.id, durId: null});
      }
    } else {
      for (const c of snap.categories) {
        for (const d of snap.durations) {
          out.push({catId: c.id, durId: d.id});
        }
      }
    }
    return out;
  }, [snap.categories, snap.durations]);

  if (snap.phases.length === 0 || snap.categories.length === 0) {
    return (
      <p className="text-xs text-neutral-500">
        Lege zuerst Phasen und Kategorien an, dann erscheint hier die Preismatrix.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-xs font-medium text-neutral-500 uppercase tracking-wide">Preismatrix ({snap.currency})</h3>
      <div className="overflow-x-auto">
        <table className="text-sm w-full">
          <thead>
            <tr className="text-left text-neutral-500">
              <th className="py-2 font-normal">Kategorie / Dauer</th>
              {snap.phases.map((p) => (
                <th key={p.id} className="py-2 font-normal">
                  {p.name}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const cat = snap.categories.find((c) => c.id === r.catId)!;
              const dur = r.durId ? snap.durations.find((d) => d.id === r.durId) : null;
              return (
                <tr key={r.catId + ':' + (r.durId ?? '_')} className="border-t border-neutral-100 dark:border-neutral-900">
                  <td className="py-2 pr-4">
                    {cat.name}
                    {dur && <span className="text-neutral-500"> · {dur.name}</span>}
                  </td>
                  {snap.phases.map((ph) => {
                    const price = lookup.get(priceKey(ph.id, r.catId, r.durId));
                    return (
                      <td key={ph.id} className="py-1 pr-4">
                        <PriceCell
                          eventId={eventId}
                          phaseId={ph.id}
                          categoryId={r.catId}
                          durationId={r.durId}
                          existing={price}
                          onChange={onChange}
                        />
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function priceKey(phaseId: string, catId: string, durId: string | null): string {
  return phaseId + '|' + catId + '|' + (durId ?? '');
}

function PriceCell({
  eventId,
  phaseId,
  categoryId,
  durationId,
  existing,
  onChange,
}: {
  eventId: string;
  phaseId: string;
  categoryId: string;
  durationId: string | null;
  existing: Price | undefined;
  onChange: () => Promise<void>;
}) {
  const [value, setValue] = useState(existing ? centsToDisplay(existing.amount_minor) : '');
  const [state, setState] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');

  useEffect(() => {
    setValue(existing ? centsToDisplay(existing.amount_minor) : '');
    setState('idle');
  }, [existing?.id, existing?.amount_minor]);

  async function commit() {
    if (state === 'saving') return;
    const initial = existing ? centsToDisplay(existing.amount_minor) : '';
    if (value.trim() === initial) return; // no-op

    setState('saving');
    try {
      if (value.trim() === '') {
        if (existing) {
          await api(`/api/admin/events/${eventId}/pricing/prices/${existing.id}`, {method: 'DELETE'});
        }
      } else {
        const cents = displayToCents(value);
        if (cents === null) {
          setState('error');
          return;
        }
        await api(`/api/admin/events/${eventId}/pricing/prices`, {
          method: 'PUT',
          body: {phase_id: phaseId, category_id: categoryId, duration_id: durationId, amount_minor: cents},
        });
      }
      await onChange();
      setState('saved');
      setTimeout(() => setState((s) => (s === 'saved' ? 'idle' : s)), 1200);
    } catch {
      setState('error');
    }
  }

  return (
    <div className="flex items-center gap-1">
      <input
        type="text"
        inputMode="decimal"
        placeholder="—"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
        }}
        className={
          'w-24 border rounded px-2 py-1 bg-transparent ' +
          (state === 'error'
            ? 'border-red-500'
            : state === 'saved'
              ? 'border-emerald-500'
              : 'border-neutral-300 dark:border-neutral-700')
        }
      />
      {state === 'saving' && <span className="text-xs text-neutral-400">…</span>}
    </div>
  );
}

// ============================================================================
// Coupons block
// ============================================================================

function CouponsBlock({eventId, coupons, onChange}: {eventId: string; coupons: Coupon[]; onChange: () => Promise<void>}) {
  const [code, setCode] = useState('');
  const [type, setType] = useState<'fixed_reduce' | 'percental_reduce' | 'guestlist'>('fixed_reduce');
  const [value, setValue] = useState('');
  const [maxUses, setMaxUses] = useState('');
  const [singleEmail, setSingleEmail] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function add() {
    setBusy(true);
    setErr(null);
    try {
      const body: Record<string, unknown> = {
        code,
        type,
        single_use_per_email: singleEmail,
      };
      if (type === 'fixed_reduce') {
        const cents = displayToCents(value);
        if (cents === null) {
          setErr('invalid_value');
          return;
        }
        body.value_minor = cents;
      } else if (type === 'percental_reduce') {
        const pct = Number(value);
        if (!Number.isFinite(pct) || pct <= 0 || pct > 100) {
          setErr('invalid_percent');
          return;
        }
        body.value_percent = Math.round(pct);
      }
      if (maxUses.trim() !== '') {
        const n = Number(maxUses);
        if (Number.isFinite(n) && n > 0) body.max_uses = Math.round(n);
      }
      await api(`/api/admin/events/${eventId}/pricing/coupons`, {method: 'POST', body});
      setCode('');
      setValue('');
      setMaxUses('');
      setSingleEmail(false);
      await onChange();
    } catch (e) {
      setErr(e instanceof APIError ? e.code : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    if (!confirm('Coupon löschen?')) return;
    await api(`/api/admin/events/${eventId}/pricing/coupons/${id}`, {method: 'DELETE'});
    await onChange();
  }

  return (
    <div className="flex flex-col gap-3 border-t border-neutral-200 dark:border-neutral-800 pt-4">
      <h3 className="text-xs font-medium text-neutral-500 uppercase tracking-wide">Coupons</h3>
      {coupons.length === 0 && <p className="text-xs text-neutral-500">Noch keine Coupons.</p>}
      {coupons.length > 0 && (
        <ul className="text-sm flex flex-col gap-1">
          {coupons.map((c) => (
            <li
              key={c.id}
              className="flex items-center justify-between border-b border-neutral-100 dark:border-neutral-900 py-1"
            >
              <span>
                <strong>{c.code}</strong>{' '}
                <span className="text-xs text-neutral-500">
                  {c.type === 'fixed_reduce' && c.value_minor !== null
                    ? `−${centsToDisplay(c.value_minor)}`
                    : c.type === 'percental_reduce'
                      ? `−${c.value_percent}%`
                      : 'Gästeliste'}
                  {c.max_uses !== null && ` · max ${c.max_uses}×`}
                  {c.single_use_per_email && ' · 1× pro E-Mail'}
                </span>
              </span>
              <button onClick={() => remove(c.id)} className="text-xs text-red-600 hover:underline">
                löschen
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="grid grid-cols-1 md:grid-cols-5 gap-2 text-sm items-end">
        <input
          type="text"
          placeholder="Code"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          className={inputCls}
        />
        <select value={type} onChange={(e) => setType(e.target.value as typeof type)} className={inputCls}>
          <option value="fixed_reduce">Festbetrag</option>
          <option value="percental_reduce">Prozent</option>
          <option value="guestlist">Gästeliste</option>
        </select>
        <input
          type="text"
          placeholder={type === 'percental_reduce' ? 'z. B. 15' : type === 'fixed_reduce' ? 'z. B. 5.00' : '—'}
          value={value}
          disabled={type === 'guestlist'}
          onChange={(e) => setValue(e.target.value)}
          className={inputCls + ' disabled:opacity-50'}
        />
        <input
          type="number"
          placeholder="max Uses"
          value={maxUses}
          onChange={(e) => setMaxUses(e.target.value)}
          className={inputCls}
        />
        <label className="flex items-center gap-2 text-xs">
          <input type="checkbox" checked={singleEmail} onChange={(e) => setSingleEmail(e.target.checked)} />
          1× pro E-Mail
        </label>
      </div>
      <div className="flex items-center gap-3">
        <button onClick={add} disabled={busy || !code || (type !== 'guestlist' && !value)} className={btnSmall}>
          Coupon erstellen
        </button>
        {err && <p className="text-xs text-red-600">{err}</p>}
      </div>
    </div>
  );
}

// ============================================================================
// Shared classnames
// ============================================================================

const inputCls = 'border border-neutral-300 dark:border-neutral-700 rounded px-2 py-1 bg-transparent text-sm';

const btnPrimary =
  'bg-neutral-900 text-white dark:bg-neutral-100 dark:text-neutral-900 rounded px-3 py-1.5 text-sm disabled:opacity-50';

const btnSmall =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-1 hover:bg-neutral-100 dark:hover:bg-neutral-800 text-sm disabled:opacity-50';
