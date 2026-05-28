'use client';

import {useEffect, useState} from 'react';
import {api, APIError} from '@/lib/api';

interface AdminUser {
  id: string;
  email: string;
  role: string;
  display_name: string;
  active: boolean;
  disabled: boolean;
  created_at: string;
  last_login_at?: string;
  sessions_count: number;
}

const ROLES = [
  {value: 'global_admin', label: 'Globaler Admin'},
  {value: 'event_admin', label: 'Event-Admin'},
  {value: 'event_manager', label: 'Event-Manager'},
];

function roleLabel(r: string): string {
  return ROLES.find((x) => x.value === r)?.label ?? r;
}

function fmtDate(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return d.toLocaleString('de-DE', {dateStyle: 'short', timeStyle: 'short'});
}

export default function AdminUsersPage() {
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [meId, setMeId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // invite form state
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState('event_admin');
  const [inviteName, setInviteName] = useState('');
  const [inviting, setInviting] = useState(false);
  const [inviteMsg, setInviteMsg] = useState<string | null>(null);

  async function load() {
    try {
      const r = await api<{users: AdminUser[]}>('/api/admin/users');
      setUsers(r.users);
      setError(null);
    } catch (err) {
      if (err instanceof APIError && err.status === 403) {
        setError('Du brauchst die globale Admin-Rolle, um diese Seite zu sehen.');
      } else {
        setError(String(err));
      }
    }
  }

  useEffect(() => {
    load();
    // Stash my own id so we can hide the "Anmelden als" button on my row.
    api<{id: string}>('/api/auth/me').then((me) => setMeId(me.id)).catch(() => {});
  }, []);

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault();
    setInviting(true);
    setInviteMsg(null);
    try {
      await api('/api/admin/users', {
        method: 'POST',
        body: {email: inviteEmail.trim().toLowerCase(), role: inviteRole, display_name: inviteName.trim()},
      });
      setInviteEmail('');
      setInviteName('');
      setInviteMsg('Einladung gesendet.');
      await load();
    } catch (err) {
      if (err instanceof APIError) {
        setInviteMsg(err.code + ': ' + err.developerMessage);
      } else {
        setInviteMsg(String(err));
      }
    } finally {
      setInviting(false);
    }
  }

  async function postAction(id: string, path: string, body?: unknown) {
    try {
      await api(`/api/admin/users/${id}/${path}`, {method: 'POST', body});
      await load();
    } catch (err) {
      alert(err instanceof APIError ? err.code + ': ' + err.developerMessage : String(err));
    }
  }

  async function impersonate(id: string, email: string) {
    if (!confirm(`Als ${email} anmelden? Du arbeitest danach mit deren Berechtigungen — bis du oben auf "Zurück zu mir" klickst.`)) return;
    try {
      await api(`/api/admin/users/${id}/impersonate`, {method: 'POST'});
      // Hard reload so the layout's /me fetch picks up the new session +
      // shows the amber impersonation banner across the whole app.
      window.location.href = '/de/admin/events';
    } catch (err) {
      alert(err instanceof APIError ? err.code + ': ' + err.developerMessage : String(err));
    }
  }

  async function resendInvite(id: string, email: string) {
    if (!confirm(`Neue Einladung an ${email} senden? Der alte Setup-Link wird ungültig.`)) return;
    try {
      await api(`/api/admin/users/${id}/resend-invite`, {method: 'POST'});
      alert(`Einladung erneut an ${email} gesendet (7 Tage gültig).`);
      await load();
    } catch (err) {
      alert(err instanceof APIError ? err.code + ': ' + err.developerMessage : String(err));
    }
  }

  async function setPassword(id: string, email: string) {
    const pw = prompt(
      `Neues Passwort für ${email} (mind. 12 Zeichen). Die Person wird auf allen Geräten abgemeldet.`
    );
    if (!pw) return;
    if (pw.length < 12) {
      alert('Passwort muss mindestens 12 Zeichen lang sein.');
      return;
    }
    const reason = prompt('Grund (für Audit-Log, optional):', '') ?? '';
    try {
      await api(`/api/admin/users/${id}/set-password`, {
        method: 'POST',
        body: {new_password: pw, reason},
      });
      alert(`Passwort gesetzt. Übergib es ${email} sicher (außerhalb dieser App).`);
      await load();
    } catch (err) {
      alert(err instanceof APIError ? err.code + ': ' + err.developerMessage : String(err));
    }
  }

  return (
    <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10 flex flex-col gap-8">
      <header>
        <h1 className="text-2xl font-light tracking-wide">Admin-Verwaltung</h1>
        <p className="text-sm opacity-70 mt-1">
          Lade weitere Admins ein, ändere Rollen oder deaktiviere Accounts. Eingeladene Personen
          bekommen einen Setup-Link per E-Mail (7 Tage gültig) und legen dort ihr Passwort selbst fest.
        </p>
      </header>

      {error && (
        <div className="rounded-md border border-red-300 bg-red-50 dark:bg-red-950 dark:border-red-800 p-3 text-sm text-red-800 dark:text-red-200">
          {error}
        </div>
      )}

      {/* Invite form */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h2 className="text-base font-medium mb-3">Neuen Admin einladen</h2>
        <form onSubmit={handleInvite} className="grid grid-cols-1 sm:grid-cols-4 gap-3 items-end">
          <label className="flex flex-col gap-1 sm:col-span-2">
            <span className="text-xs uppercase tracking-wide opacity-70">E-Mail</span>
            <input
              type="email"
              required
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Rolle</span>
            <select
              value={inviteRole}
              onChange={(e) => setInviteRole(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            >
              {ROLES.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs uppercase tracking-wide opacity-70">Anzeigename (optional)</span>
            <input
              value={inviteName}
              onChange={(e) => setInviteName(e.target.value)}
              className="rounded-md border border-neutral-300 dark:border-neutral-700 bg-transparent px-3 py-2 text-sm"
            />
          </label>
          <button
            type="submit"
            disabled={inviting}
            className="sm:col-span-4 sm:justify-self-end rounded-md bg-neutral-900 dark:bg-neutral-100 text-neutral-100 dark:text-neutral-900 px-4 py-2 text-sm disabled:opacity-50"
          >
            {inviting ? 'Sende…' : 'Einladung senden'}
          </button>
        </form>
        {inviteMsg && <p className="mt-3 text-sm">{inviteMsg}</p>}
      </section>

      {/* User list */}
      <section>
        {users === null && !error && <p className="text-sm opacity-70">Lädt…</p>}
        {users && (
          <table className="w-full text-sm">
            <thead className="text-left border-b border-neutral-200 dark:border-neutral-800">
              <tr>
                <th className="py-2 font-medium">E-Mail</th>
                <th className="py-2 font-medium">Rolle</th>
                <th className="py-2 font-medium">Status</th>
                <th className="py-2 font-medium">Letzter Login</th>
                <th className="py-2 font-medium">Sessions</th>
                <th className="py-2 font-medium text-right">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className="border-b border-neutral-100 dark:border-neutral-900 align-top">
                  <td className="py-3">
                    <div>{u.email}</div>
                    {u.display_name && <div className="text-xs opacity-60">{u.display_name}</div>}
                  </td>
                  <td className="py-3">
                    <select
                      defaultValue={u.role}
                      onChange={(e) => postAction(u.id, 'role', {role: e.target.value})}
                      className="bg-transparent border border-neutral-300 dark:border-neutral-700 rounded px-2 py-1 text-xs"
                    >
                      {ROLES.map((r) => (
                        <option key={r.value} value={r.value}>
                          {r.label}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="py-3">
                    {u.disabled ? (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-200">
                        deaktiviert
                      </span>
                    ) : !u.active ? (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
                        Setup ausstehend
                      </span>
                    ) : (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200">
                        aktiv
                      </span>
                    )}
                  </td>
                  <td className="py-3 text-xs opacity-70">{fmtDate(u.last_login_at)}</td>
                  <td className="py-3 text-xs opacity-70">{u.sessions_count}</td>
                  <td className="py-3 text-right">
                    <div className="inline-flex gap-2 text-xs">
                      {u.disabled ? (
                        <button onClick={() => postAction(u.id, 'reactivate')} className="hover:underline">
                          Reaktivieren
                        </button>
                      ) : (
                        <>
                          {!u.active && (
                            <button
                              onClick={() => resendInvite(u.id, u.email)}
                              className="hover:underline"
                              title="Neuen Setup-Link generieren und per E-Mail schicken (alter Link wird ungültig)"
                            >
                              Einladung erneut senden
                            </button>
                          )}
                          {u.id !== meId && u.role !== 'global_admin' && u.active && (
                            <button
                              onClick={() => impersonate(u.id, u.email)}
                              className="hover:underline"
                              title="Mit den Berechtigungen dieser Person arbeiten — Zurück über den Banner oben"
                            >
                              Anmelden als
                            </button>
                          )}
                          {u.id !== meId && (
                            <button
                              onClick={() => setPassword(u.id, u.email)}
                              className="hover:underline"
                              title="Passwort direkt setzen — die Person wird überall abgemeldet und du musst das neue Passwort übergeben"
                            >
                              Passwort setzen
                            </button>
                          )}
                          <button
                            onClick={() => postAction(u.id, 'force-logout')}
                            className="hover:underline"
                            title="Alle Sessions dieses Users beenden"
                          >
                            Force-Logout
                          </button>
                          <button
                            onClick={() => {
                              if (confirm(`Account ${u.email} deaktivieren?`)) {
                                postAction(u.id, 'deactivate');
                              }
                            }}
                            className="hover:underline text-red-700 dark:text-red-300"
                          >
                            Deaktivieren
                          </button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
