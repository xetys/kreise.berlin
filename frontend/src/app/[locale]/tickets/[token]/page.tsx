'use client';

import {use, useEffect, useState} from 'react';
import {usePathname, useRouter} from 'next/navigation';
import {api, APIError} from '@/lib/api';

interface TicketDTO {
  ticket_id: string;
  status: 'booked' | 'paid' | 'canceled' | 'checked_in';
  holder_name: string;
  holder_email?: string;
  amount_minor: number;
  currency: string;
  phase_name?: string;
  category_name?: string;
  duration_name?: string;
  has_qr: boolean;
  qr_url?: string;
  can_cancel: boolean;
  can_transfer: boolean;
  event: {
    name: string;
    slug: string;
    starts_at: string;
    ends_at: string;
    location: string;
    color_primary: string;
    color_secondary: string;
    color_text: string;
  };
  booking_contact: string;
  checked_in_at?: string;
  canceled_at?: string;
}

const statusLabels: Record<string, string> = {
  booked: 'reserviert',
  paid: 'bezahlt',
  canceled: 'storniert',
  checked_in: 'eingecheckt',
};

const errorMessages: Record<string, string> = {
  invalid_token: 'Dieser Link ist ungültig.',
  ticket_not_found: 'Ticket nicht gefunden.',
  stale_token: 'Dieser Link wurde durch eine Übertragung ersetzt.',
  cannot_cancel: 'Dieses Ticket kann nicht storniert werden.',
  cannot_transfer: 'Dieses Ticket kann nicht übertragen werden.',
  qr_unavailable: 'Der QR-Code ist erst nach Bezahlung verfügbar.',
};

function translate(code: string): string {
  return errorMessages[code] ?? `Fehler: ${code}`;
}

function formatRange(startsISO: string, endsISO: string): string {
  const s = new Date(startsISO);
  const e = new Date(endsISO);
  const opts: Intl.DateTimeFormatOptions = {day: '2-digit', month: 'long', year: 'numeric'};
  return `${s.toLocaleDateString('de-DE', opts)} – ${e.toLocaleDateString('de-DE', opts)}`;
}

function formatMoney(minor: number, currency: string): string {
  const whole = Math.floor(minor / 100);
  const cents = Math.abs(minor % 100).toString().padStart(2, '0');
  return `${whole},${cents} ${currency}`;
}

interface PageProps {
  params: Promise<{locale: string; token: string}>;
}

export default function TicketPage({params}: PageProps) {
  const {token} = use(params);
  const pathname = usePathname();
  const router = useRouter();
  const localePrefix = pathname.split('/')[1] ?? 'de';

  const [ticket, setTicket] = useState<TicketDTO | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [showTransfer, setShowTransfer] = useState(false);

  async function load() {
    try {
      const t = await api<TicketDTO>(`/api/tickets/${token}`);
      setTicket(t);
      setError(null);
    } catch (e) {
      if (e instanceof APIError) setError(translate(e.code));
      else setError(String(e));
    }
  }

  useEffect(() => {
    load();
  }, [token]);

  if (error) {
    return (
      <main className="flex-1 max-w-2xl mx-auto w-full px-6 py-16 text-center">
        <h1 className="text-2xl font-semibold mb-4">Ticket nicht verfügbar</h1>
        <p className="text-neutral-600">{error}</p>
      </main>
    );
  }
  if (!ticket) {
    return <p className="text-sm text-neutral-500 px-6 py-10">Lädt…</p>;
  }

  const themeStyle: React.CSSProperties = {
    ['--ev-primary' as string]: ticket.event.color_primary,
    ['--ev-secondary' as string]: ticket.event.color_secondary,
    ['--ev-text' as string]: ticket.event.color_text,
  };

  async function cancel() {
    if (!confirm('Ticket wirklich stornieren? Das kann nicht rückgängig gemacht werden.')) return;
    try {
      await api(`/api/tickets/${token}/cancel`, {method: 'POST'});
      setActionMsg('Ticket storniert.');
      await load();
    } catch (e) {
      setActionMsg(e instanceof APIError ? translate(e.code) : String(e));
    }
  }

  return (
    <div className="flex-1 flex flex-col" style={themeStyle}>
      <header
        className="w-full py-8 px-6"
        style={{backgroundColor: ticket.event.color_primary, color: ticket.event.color_text}}
      >
        <div className="max-w-2xl mx-auto flex flex-col gap-1">
          <p className="text-sm opacity-80">Ticket für</p>
          <h1 className="text-3xl font-semibold">{ticket.event.name}</h1>
          <p className="text-base opacity-90">{formatRange(ticket.event.starts_at, ticket.event.ends_at)}</p>
          {ticket.event.location && <p className="text-base opacity-80">{ticket.event.location}</p>}
        </div>
      </header>

      <main className="max-w-2xl mx-auto w-full px-6 py-8 flex flex-col gap-8">
        {actionMsg && <p className="text-sm text-emerald-700">{actionMsg}</p>}

        <section className="border border-neutral-200 dark:border-neutral-800 rounded p-4 flex flex-col gap-2">
          <div className="flex justify-between items-center">
            <span className="text-sm text-neutral-500">Status</span>
            <span className="text-sm font-medium">{statusLabels[ticket.status] ?? ticket.status}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm text-neutral-500">Inhaber</span>
            <span className="text-sm font-medium">{ticket.holder_name}</span>
          </div>
          {(ticket.category_name || ticket.duration_name) && (
            <div className="flex justify-between">
              <span className="text-sm text-neutral-500">Kategorie</span>
              <span className="text-sm font-medium">
                {ticket.category_name}
                {ticket.duration_name ? ` · ${ticket.duration_name}` : ''}
                {ticket.phase_name ? ` (${ticket.phase_name})` : ''}
              </span>
            </div>
          )}
          <div className="flex justify-between">
            <span className="text-sm text-neutral-500">Betrag</span>
            <span className="text-sm font-medium">{formatMoney(ticket.amount_minor, ticket.currency)}</span>
          </div>
        </section>

        {ticket.has_qr && ticket.qr_url && (
          <section className="flex flex-col items-center gap-2 border border-neutral-200 dark:border-neutral-800 rounded p-6">
            <h2 className="text-sm font-medium">QR-Code</h2>
            <img src={ticket.qr_url} alt="" className="w-64 h-64 bg-white p-2 rounded" />
            <p className="text-xs text-neutral-500 text-center">
              Bei Eintritt scannen lassen. Bitte nicht weitergeben.
            </p>
          </section>
        )}

        {!ticket.has_qr && ticket.status === 'booked' && (
          <p className="text-sm text-neutral-500 italic">
            Der QR-Code erscheint hier, sobald deine Zahlung bestätigt wurde.
          </p>
        )}

        <section className="flex flex-wrap gap-3">
          {ticket.can_cancel && (
            <button onClick={cancel} className={btnDanger}>
              Stornieren
            </button>
          )}
          {ticket.can_transfer && (
            <button onClick={() => setShowTransfer(true)} className={btnSecondary}>
              Übertragen
            </button>
          )}
        </section>

        {showTransfer && (
          <TransferForm
            token={token}
            onCanceled={() => setShowTransfer(false)}
            onTransferred={(newURL) => {
              router.replace(newURL);
            }}
          />
        )}

        <p className="text-xs text-neutral-500 mt-4 border-t border-neutral-100 dark:border-neutral-900 pt-4">
          Buchung gekoppelt an: {ticket.booking_contact}
          {ticket.canceled_at && (
            <>
              {' '}· storniert am {new Date(ticket.canceled_at).toLocaleString('de-DE')}
            </>
          )}
          {ticket.checked_in_at && (
            <>
              {' '}· eingecheckt am {new Date(ticket.checked_in_at).toLocaleString('de-DE')}
            </>
          )}
        </p>
      </main>
    </div>
  );

  void localePrefix; // currently unused; kept for future locale-scoped links.
}

function TransferForm({
  token,
  onCanceled,
  onTransferred,
}: {
  token: string;
  onCanceled: () => void;
  onTransferred: (newURL: string) => void;
}) {
  const pathname = usePathname();
  const localePrefix = pathname.split('/')[1] ?? 'de';
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const resp = await api<{new_view_token: string; new_view_url?: string}>(
        `/api/tickets/${token}/transfer`,
        {method: 'POST', body: {new_name: name, new_email: email}}
      );
      const url = resp.new_view_url || `/${localePrefix}/tickets/${resp.new_view_token}`;
      onTransferred(url);
    } catch (e) {
      setErr(e instanceof APIError ? translate(e.code) : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={submit}
      className="flex flex-col gap-3 border border-neutral-200 dark:border-neutral-800 rounded p-4"
    >
      <h2 className="text-sm font-medium">Ticket übertragen</h2>
      <p className="text-xs text-neutral-500">
        Der bisherige Link wird ungültig. Den neuen Link bekommt der/die neue Inhaber:in.
      </p>
      <input
        required
        placeholder="Neuer Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className={inputCls}
      />
      <input
        type="email"
        placeholder="Neue E-Mail (optional)"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        className={inputCls}
      />
      {err && <p className="text-xs text-red-600">{err}</p>}
      <div className="flex gap-2">
        <button type="submit" disabled={busy || !name} className={btnSecondary}>
          {busy ? 'Übertragen…' : 'Übertragen'}
        </button>
        <button type="button" onClick={onCanceled} className="text-sm text-neutral-500 hover:underline">
          Abbrechen
        </button>
      </div>
    </form>
  );
}

const btnDanger =
  'border border-red-300 dark:border-red-700 text-red-700 dark:text-red-300 rounded px-4 py-2 text-sm hover:bg-red-50 dark:hover:bg-red-900/30';

const btnSecondary =
  'border border-neutral-300 dark:border-neutral-700 rounded px-4 py-2 text-sm hover:bg-neutral-100 dark:hover:bg-neutral-800';

const inputCls =
  'border border-neutral-300 dark:border-neutral-700 rounded px-3 py-2 bg-transparent text-sm';
