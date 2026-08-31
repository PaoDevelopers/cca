package web

import (
	"html/template"
	"log/slog"
	"net/http"
)

// The only server-rendered pages: the two sign-in pages. Everything
// else is an SPA or an API.

// Mirrors ui/common/styles/base.css by hand; these pages stay
// independent of the SPA build.
const pageStyle = `
:root {
	color-scheme: light dark;
	--fg: light-dark(oklch(0 0 0), oklch(0.952 0.003 264.5));
	--bg: light-dark(oklch(0.99 0 0), oklch(0.239 0.004 264.5));
	--danger: light-dark(oklch(0.55 0.19 27), oklch(0.72 0.16 27));
}
html {
	box-sizing: border-box;
	color: var(--fg);
	background-color: var(--bg);
}
*, *::before, *::after {
	box-sizing: inherit;
	line-height: calc(1em + 0.5rem);
}
body {
	display: flex;
	flex-direction: column;
	min-height: 100vh;
	font-family: sans-serif;
	max-width: 24rem;
	margin: 0 auto;
	padding: 4rem 1rem 1rem;

	> *:has(+ footer) {
		flex: 1;
	}

	> footer {
		margin-top: 1rem;
		font-size: 0.75rem;
		color: color-mix(in oklab, var(--fg) 65%, var(--bg));

		a:link, a:visited {
			color: inherit;
			text-decoration-color: color-mix(in oklab, currentColor 55%, transparent);
		}
	}
}
a:link, a:visited {
	text-decoration-thickness: 1.89px;
	text-decoration-color: light-dark(oklch(0.85 0 0), oklch(0.42 0 0));
	color: light-dark(oklch(0.428 0.088 249.2), oklch(0.78 0.115 248.6));
}
a.button {
	display: inline-block;
	padding: 0.5rem 1rem;
	/*
	 * The only action on the page, and its outline was the only thing
	 * marking it as one: 2.7:1 in light and 2.2:1 in dark against the
	 * page, both under the 3:1 a non-text boundary needs, over a
	 * background 1.1:1 from the page behind it. Taken to a ratio that
	 * holds in both schemes, with a surface that is visible on its own.
	 */
	border: 1px solid color-mix(in oklab, var(--bg) 45%, var(--fg));
	background-color: color-mix(in oklab, var(--bg) 92%, var(--fg));
	color: var(--fg);
	text-decoration: none;
}
.error {
	border: 1px solid var(--danger);
	background-color: color-mix(in oklab, var(--bg) 92%, var(--danger));
	padding: 0.75rem;
}
small {
	color: color-mix(in oklab, var(--fg) 65%, var(--bg));
}
`

const pageFooter = `<footer>
<a href="https://github.com/PaoDevelopers/cca">YK Pao School CCAs</a>
is licensed under
<a href="https://spdx.org/licenses/AGPL-3.0-only.html">GNU Affero General Public License v3.0 only</a>.
</footer>`

//nolint:gochecknoglobals
var signinTemplate = template.Must(template.New("signin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>CCA {{ .Role }} sign-in</title>
<style>` + pageStyle + `</style>
</head>
<body>
<main>
<h1>CCA {{ .Role }} area</h1>
{{ if .Error }}<p class="error">{{ .Error }}</p>{{ end }}
<p><a class="button" href="{{ .URL }}">Sign in with your school account</a></p>
<p><small><a href="/">Back</a></small></p>
</main>` + pageFooter + `
</body>
</html>
`))

type signinData struct {
	Role  string
	URL   string
	Error string
}

func (app *Server) renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if err := tmpl.Execute(w, data); err != nil {
		app.logError(r, logMsgHTTPResponseError, slog.Any("error", err))
	}
}
