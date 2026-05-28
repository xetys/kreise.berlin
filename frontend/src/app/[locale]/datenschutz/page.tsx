import {PublicShell} from '@/components/PublicShell';

export const metadata = {
  title: 'Datenschutzerklärung · kreise.berlin',
};

export default async function DatenschutzPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  return (
    <PublicShell locale={locale}>
      <article className="prose prose-neutral dark:prose-invert max-w-none">
        <h1 className="text-2xl sm:text-3xl font-light tracking-wide">Datenschutzerklärung</h1>
        <p className="text-sm opacity-70 mt-2">
          Information gemäß Art. 13 DSGVO. Diese Erklärung beschreibt, welche personenbezogenen
          Daten beim Buchen von Tickets über diese Plattform verarbeitet werden.
        </p>

        <section className="mt-8 flex flex-col gap-6 text-sm leading-relaxed">
          <Block label="1. Verantwortliche Stelle">
            <p>
              Verantwortlich im Sinne des Art. 4 Nr. 7 DSGVO für die Datenverarbeitung auf dieser
              Website ist:
            </p>
            <p>
              David Steiman
              <br />
              Rohrdamm 65A
              <br />
              13629 Berlin
              <br />
              Deutschland
            </p>
            <p>
              Datenschutzanfragen per E-Mail an{' '}
              <a href="mailto:adinatbust@gmail.com" className="underline">
                adinatbust@gmail.com
              </a>
              .
            </p>
          </Block>

          <Block label="2. Allgemeines zur Datenverarbeitung">
            <p>
              Wir verarbeiten personenbezogene Daten unserer Nutzer:innen grundsätzlich nur, soweit
              dies zur Bereitstellung einer funktionsfähigen Website sowie unserer Inhalte und
              Leistungen erforderlich ist. Die Verarbeitung erfolgt regelmäßig nur nach Einwilligung
              der Nutzer:innen oder auf Grundlage einer gesetzlichen Erlaubnis (Art. 6 DSGVO).
            </p>
          </Block>

          <Block label="3. Beim Aufruf der Website">
            <p>
              Beim Aufruf werden vom Webserver technische Verbindungsdaten verarbeitet (IP-Adresse,
              Datum/Uhrzeit, Browser/Betriebssystem, aufgerufene Seite). Rechtsgrundlage ist Art. 6
              Abs. 1 lit. f DSGVO; das berechtigte Interesse besteht im sicheren Betrieb der Seite.
              Die Server-Logs werden nach maximal 30 Tagen gelöscht.
            </p>
          </Block>

          <Block label="4. Cookies">
            <p>
              Diese Website nutzt ausschließlich technisch notwendige Cookies. Es werden keine
              Tracking-, Analyse- oder Werbe-Cookies eingesetzt. Eine Einwilligung nach § 25 TTDSG
              ist daher nicht erforderlich.
            </p>
          </Block>

          <Block label="5. Ticket-Buchung">
            <p>
              Bei einer Ticketbuchung verarbeiten wir folgende Daten, soweit Sie sie angeben:
            </p>
            <ul className="list-disc ml-5 flex flex-col gap-1">
              <li>Vor- und Nachname (Kontakt sowie pro Teilnehmer:in)</li>
              <li>E-Mail-Adresse</li>
              <li>ggf. Anschrift bei Banküberweisung</li>
              <li>Buchungsdetails (Ticketart, Anzahl, Betrag, Veranstaltung)</li>
              <li>Zahlungsmethode (Banküberweisung, PayPal-Verweis, Vor-Ort-Zahlung, Spende)</li>
            </ul>
            <p>
              Rechtsgrundlage ist Art. 6 Abs. 1 lit. b DSGVO (Vertragserfüllung) sowie für
              gesetzliche Aufbewahrungspflichten Art. 6 Abs. 1 lit. c DSGVO. Buchungsdaten werden
              nach Ablauf der handels- und steuerrechtlichen Aufbewahrungsfristen gelöscht.
            </p>
          </Block>

          <Block label="6. Versand der Buchungsbestätigungen / Tickets">
            <p>
              Buchungs- und Ticketmails werden über Amazon Web Services (AWS) Simple Email Service
              (SES) versendet. Anbieter ist Amazon Web Services EMEA SARL, 38 avenue John F.
              Kennedy, L-1855 Luxembourg. Mit AWS besteht ein Vertrag zur Auftragsverarbeitung gemäß
              Art. 28 DSGVO.
            </p>
            <p>
              Mit Amazon Web Services besteht ein Auftragsverarbeitungsvertrag gemäß
              Art. 28 DSGVO (AWS GDPR Data Processing Addendum).
            </p>
          </Block>

          <Block label="7. Hosting & Speicherung">
            <p>
              Diese Website wird auf einem Kubernetes-Cluster bei der SysEleven GmbH,
              Boxhagener Straße 80, 10245 Berlin gehostet (MetaKube). Banner- und
              Mediendateien sowie alle personenbezogenen Buchungsdaten liegen im
              gleichen Cluster (PostgreSQL und MinIO als interne Komponenten). Die
              Daten verlassen den deutschen Hosting-Standort nicht.
            </p>
            <p>
              Mit der SysEleven GmbH besteht ein Auftragsverarbeitungsvertrag gemäß
              Art. 28 DSGVO.
            </p>
          </Block>

          <Block label="8. PayPal-Verweise">
            <p>
              Sofern für eine Veranstaltung die Zahlung per PayPal angeboten wird, werden Sie über
              einen Deep-Link zu paypal.me weitergeleitet. Die Zahlung erfolgt direkt zwischen Ihnen
              und PayPal (Europe) S.à r.l. et Cie, S.C.A., 22-24 Boulevard Royal, L-2449 Luxembourg.
              Es gelten die Datenschutzbestimmungen von PayPal:{' '}
              <a
                href="https://www.paypal.com/de/legalhub/privacy-full"
                className="underline"
                rel="noreferrer"
              >
                https://www.paypal.com/de/legalhub/privacy-full
              </a>
              .
            </p>
          </Block>

          <Block label="9. Karten / OpenStreetMap">
            <p>
              Auf Veranstaltungsseiten zeigen wir den Veranstaltungsort auf einer Karte. Dabei
              werden zwei Anfragen an Server der OpenStreetMap-Stiftung übermittelt: eine
              Geocoding-Anfrage an Nominatim (
              <a
                href="https://nominatim.openstreetmap.org/"
                className="underline"
                rel="noreferrer"
              >
                nominatim.openstreetmap.org
              </a>
              ), um den Ortsnamen in Koordinaten zu übersetzen, sowie ein eingebettetes
              Karten-iframe von{' '}
              <a href="https://www.openstreetmap.org/" className="underline" rel="noreferrer">
                openstreetmap.org
              </a>
              . Dabei werden Ihre IP-Adresse, Browser-Informationen und der angefragte Ort an die
              OpenStreetMap-Server übertragen. Die Geocoding-Antwort wird in Ihrem Browser
              (localStorage) zwischengespeichert, sodass derselbe Ort nicht erneut abgefragt wird.
              Rechtsgrundlage ist Art. 6 Abs. 1 lit. f DSGVO; das berechtigte Interesse besteht in
              einer komfortablen Anreiseinformation. Datenschutzbestimmungen der OpenStreetMap
              Foundation:{' '}
              <a
                href="https://wiki.osmfoundation.org/wiki/Privacy_Policy"
                className="underline"
                rel="noreferrer"
              >
                wiki.osmfoundation.org/wiki/Privacy_Policy
              </a>
              .
            </p>
          </Block>

          <Block label="10. Ihre Rechte">
            <p>Sie haben jederzeit das Recht auf:</p>
            <ul className="list-disc ml-5 flex flex-col gap-1">
              <li>Auskunft über die zu Ihrer Person gespeicherten Daten (Art. 15 DSGVO)</li>
              <li>Berichtigung unrichtiger Daten (Art. 16 DSGVO)</li>
              <li>Löschung Ihrer Daten (Art. 17 DSGVO)</li>
              <li>Einschränkung der Verarbeitung (Art. 18 DSGVO)</li>
              <li>Datenübertragbarkeit (Art. 20 DSGVO)</li>
              <li>Widerspruch gegen die Verarbeitung (Art. 21 DSGVO)</li>
              <li>Widerruf erteilter Einwilligungen (Art. 7 Abs. 3 DSGVO)</li>
            </ul>
            <p>Anfragen richten Sie bitte an die in Abschnitt 1 genannte E-Mail-Adresse.</p>
          </Block>

          <Block label="11. Beschwerderecht">
            <p>
              Sie haben das Recht, sich bei einer Datenschutz-Aufsichtsbehörde über die Verarbeitung
              Ihrer personenbezogenen Daten zu beschweren. Zuständig in Berlin ist die Berliner
              Beauftragte für Datenschutz und Informationsfreiheit, Friedrichstraße 219, 10969
              Berlin.
            </p>
          </Block>

          <Block label="12. Stand">
            <p>9. Mai 2026</p>
          </Block>
        </section>
      </article>
    </PublicShell>
  );
}

function Block({label, children}: {label: string; children: React.ReactNode}) {
  return (
    <div>
      <h2 className="text-[11px] uppercase tracking-[0.22em] opacity-60 mb-2 font-medium">
        {label}
      </h2>
      <div className="flex flex-col gap-2">{children}</div>
    </div>
  );
}

function Todo({children}: {children: React.ReactNode}) {
  return (
    <p className="rounded border-2 border-dashed border-amber-400 bg-amber-50 dark:bg-amber-950/40 dark:border-amber-700 px-3 py-2 text-amber-900 dark:text-amber-200 text-xs">
      <span className="font-mono uppercase tracking-wider mr-2">[bitte ausfüllen]</span>
      {children}
    </p>
  );
}
