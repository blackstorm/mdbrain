/** @jsxImportSource ../../../../packages/ui/src */
import { raw } from "../../../../packages/ui/src/index";

export interface PublicVaultViewModel {
  id: string;
  name: string;
  domain: string;
  logoUrl?: string | null;
  faviconVersion?: string | null;
  customHeadHtml?: string | null;
}

export interface PublicBacklinkViewModel {
  clientId: string;
  title: string;
  description?: string | null;
}

export interface PublicNoteViewModel {
  clientId: string;
  title: string;
  htmlContent: string;
  path: string;
  updatedAt: string;
  backlinks: PublicBacklinkViewModel[];
}

function AppDocument({
  title,
  description,
  vault,
  children,
}: {
  title: string;
  description?: string | null;
  vault: PublicVaultViewModel;
  children: unknown;
}) {
  return (
    <html lang="en">
      <head>
        <meta charSet="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>{title}</title>
        {description ? <meta name="description" content={description} /> : null}
        {vault.faviconVersion ? (
          <link rel="icon" href={`/favicon.ico?v=${encodeURIComponent(vault.faviconVersion)}`} />
        ) : (
          <link rel="icon" type="image/png" href="/publics/shared/images/favicon.png" />
        )}
        <link rel="stylesheet" href="/publics/app/css/app.css" />
        <link rel="stylesheet" href="/publics/app/css/highlight-github-dark.min.css" />
        <link rel="stylesheet" href="/publics/app/katex/katex-swap.min.css" />
        {vault.customHeadHtml ? raw(vault.customHeadHtml) : null}
      </head>
      <body>
        <div id="app">
          <header class="navbar">
            <div class="navbar-content px-4 md:px-8">
              <div class="navbar-left">
                <a href="/" class="navbar-logo">
                  {vault.logoUrl ? (
                    <img src={vault.logoUrl} alt={vault.name} class="navbar-logo-img" />
                  ) : null}
                  <span>{vault.name || "Mdbrain"}</span>
                </a>
              </div>
              <div class="navbar-right">
                <small class="navbar-link">
                  Powered by{" "}
                  <a href="https://github.com/blackstorm/mdbrain" target="_blank">
                    Mdbrain
                  </a>
                </small>
              </div>
            </div>
          </header>
          {children}
        </div>
        <script src="/publics/app/js/highlight.min.js"></script>
        <script src="/publics/app/katex/katex.min.js"></script>
        <script src="/publics/shared/htmx.min.js"></script>
        <script src="/publics/app/js/app.js"></script>
      </body>
    </html>
  );
}

export function PublicHomePage({
  vault,
  notes,
}: {
  vault: PublicVaultViewModel;
  notes: Array<{ clientId: string; path: string; mtime?: string | null }>;
}) {
  return (
    <AppDocument title={`${vault.name} - Notes`} vault={vault}>
      <div class="max-w-4xl mx-auto p-6 md:p-12">
        <header class="pb-6 border-b border-gray-200 mb-8">
          <h1 class="text-3xl font-semibold text-gray-900 mb-2">{vault.name}</h1>
          <p class="text-sm text-gray-500">{vault.domain}</p>
        </header>

        <section>
          <h2 class="text-xl font-semibold text-gray-900 mb-6">Notes</h2>

          {notes.length > 0 ? (
            <div class="flex flex-col gap-2">
              {notes.map((note) => (
                <a
                  href={`/${note.clientId}`}
                  class="block p-4 border border-gray-200 bg-white hover:bg-gray-50 hover:border-gray-300 transition-colors"
                >
                  <h3 class="text-base font-medium text-gray-900 mb-1">{note.path}</h3>
                  <p class="text-sm text-gray-500">Last modified: {note.mtime ?? ""}</p>
                </a>
              ))}
            </div>
          ) : (
            <div class="text-center py-16">
              <div class="w-16 h-16 mx-auto mb-4 flex items-center justify-center border border-gray-200 rounded-lg">
                <svg class="w-8 h-8 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                  ></path>
                </svg>
              </div>
              <h3 class="text-lg font-semibold text-gray-900 mb-2">No notes</h3>
              <p class="text-sm text-gray-500">Please use the Obsidian plugin to sync your notes</p>
            </div>
          )}
        </section>
      </div>
    </AppDocument>
  );
}

export function PublicNoteFragment({ note }: { note: PublicNoteViewModel }) {
  return (
    <div
      class="note grow shrink-0 overflow-y-auto w-full md:w-[625px] md:max-w-[625px] bg-white sticky top-0"
      id={`note-${note.clientId}`}
    >
      <article class="p-4 md:p-8 prose">
        <h1>{note.title}</h1>
        {raw(note.htmlContent)}
        <p class="last-updated">Last updated: {note.updatedAt}</p>
      </article>

      {note.backlinks.length > 0 ? (
        <div class="backlinks-section px-4 md:px-8">
          <h2 class="backlinks-title">Links to this note</h2>
          <div class="backlinks-grid">
            {note.backlinks.map((backlink) => (
              <div class="backlink-item">
                <a
                  href={`/${backlink.clientId}`}
                  hx-get={`/${backlink.clientId}`}
                  hx-target="#notes"
                  hx-swap="beforeend"
                  {...{ "hx-headers": `{"X-From-Note-Id": "${note.clientId}"}` }}
                >
                  <span class="backlink-title">{backlink.title}</span>
                  <p class="backlink-desc">{backlink.description ?? ""}</p>
                </a>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function PublicNotePage({
  vault,
  notes,
  description,
}: {
  vault: PublicVaultViewModel;
  notes: PublicNoteViewModel[];
  description?: string | null;
}) {
  const pageTitle = notes.length > 0 ? `${notes[0]!.title} - ${vault.name}` : vault.name;

  return (
    <AppDocument title={pageTitle} description={description} vault={vault}>
      <div class="notes-container grow flex overflow-x-auto overflow-y-hidden" id="notes">
        {notes.map((note) => (
          <PublicNoteFragment note={note} />
        ))}
      </div>
    </AppDocument>
  );
}
