import {PublicShell} from '@/components/PublicShell';

export const metadata = {
  title: 'Impressum · kreise.berlin',
};

export default async function ImpressumPage({
  params,
}: {
  params: Promise<{locale: string}>;
}) {
  const {locale} = await params;
  return (
    <PublicShell locale={locale}>
      <article className="prose prose-neutral dark:prose-invert max-w-none">
        <h1 className="text-2xl sm:text-3xl font-light tracking-wide">Impressum</h1>

        <p className="text-sm opacity-70 mt-2">
          Angaben gemäß § 5 TMG sowie § 18 Abs. 2 MStV.
        </p>

        <section className="mt-8 flex flex-col gap-6 text-sm leading-relaxed">
          <Block label="Anbieter / Verantwortlich für den Inhalt">
            <p>
              David Steiman
              <br />
              Rohrdamm 65A
              <br />
              13629 Berlin
              <br />
              Deutschland
            </p>
          </Block>

          <Block label="Kontakt">
            <p>
              E-Mail:{' '}
              <a href="mailto:adinatbust@gmail.com" className="underline">
                adinatbust@gmail.com
              </a>
            </p>
          </Block>

          <Block label="EU-Streitschlichtung">
            <p>
              Die Europäische Kommission stellt eine Plattform zur Online-Streitbeilegung (OS)
              bereit:{' '}
              <a
                href="https://ec.europa.eu/consumers/odr/"
                className="underline"
                rel="noreferrer"
              >
                https://ec.europa.eu/consumers/odr/
              </a>
              .
            </p>
            <p>
              Wir sind nicht bereit oder verpflichtet, an Streitbeilegungsverfahren vor einer
              Verbraucherschlichtungsstelle teilzunehmen.
            </p>
          </Block>

          <Block label="Haftung für Inhalte">
            <p>
              Die Inhalte unserer Seiten wurden mit größter Sorgfalt erstellt. Für die Richtigkeit,
              Vollständigkeit und Aktualität der Inhalte können wir jedoch keine Gewähr übernehmen.
              Als Diensteanbieter sind wir gemäß § 7 Abs. 1 TMG für eigene Inhalte auf diesen Seiten
              nach den allgemeinen Gesetzen verantwortlich. Nach §§ 8 bis 10 TMG sind wir als
              Diensteanbieter jedoch nicht verpflichtet, übermittelte oder gespeicherte fremde
              Informationen zu überwachen.
            </p>
          </Block>

          <Block label="Haftung für Links">
            <p>
              Unser Angebot enthält ggf. Links zu externen Websites Dritter, auf deren Inhalte wir
              keinen Einfluss haben. Deshalb können wir für diese fremden Inhalte auch keine Gewähr
              übernehmen. Für die Inhalte der verlinkten Seiten ist stets der jeweilige Anbieter
              oder Betreiber der Seiten verantwortlich.
            </p>
          </Block>

          <Block label="Urheberrecht">
            <p>
              Die durch die Seitenbetreiber erstellten Inhalte und Werke unterliegen dem deutschen
              Urheberrecht. Vervielfältigung, Bearbeitung, Verbreitung und jede Art der Verwertung
              außerhalb der Grenzen des Urheberrechts bedürfen der schriftlichen Zustimmung des
              jeweiligen Autors bzw. Erstellers.
            </p>
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

/**
 * Visible amber placeholder. Operators MUST replace these with real values
 * before the site goes public — § 5 TMG requires the data to be present and
 * directly accessible. We render the marker on the page (not just a comment)
 * so it's impossible to miss during pre-launch review.
 */
function Todo({children}: {children: React.ReactNode}) {
  return (
    <p className="rounded border-2 border-dashed border-amber-400 bg-amber-50 dark:bg-amber-950/40 dark:border-amber-700 px-3 py-2 text-amber-900 dark:text-amber-200 text-xs">
      <span className="font-mono uppercase tracking-wider mr-2">[bitte ausfüllen]</span>
      {children}
    </p>
  );
}
