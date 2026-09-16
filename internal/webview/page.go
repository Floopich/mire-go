package webview

const pageHTML = `<!DOCTYPE html>
<html lang="fr"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relevés DOCSIS</title>
<style>
 :root { color-scheme: light dark; --trait:#c0392b; --grille:#8883; }
 body { font: 15px/1.5 system-ui, sans-serif; margin: 0 auto; padding: 16px; max-width: 780px; }
 h1 { font-size: 1.3rem; margin: 0 0 4px; }
 .meta { opacity: .7; font-size: .85rem; margin-bottom: 16px; }
 table { border-collapse: collapse; width: 100%; font-variant-numeric: tabular-nums; }
 th, td { text-align: right; padding: 5px 8px; border-bottom: 1px solid var(--grille); }
 th:first-child, td:first-child { text-align: left; }
 a { color: inherit; }
 .chart { width: 100%; height: auto; margin: 4px 0 18px; }
 .line { fill: none; stroke: var(--trait); stroke-width: 1.6; }
 .grid { stroke: var(--grille); stroke-width: 1; }
 .tick, .axis, .empty { font-size: 11px; fill: currentColor; opacity: .65; }
 .alerte { background: #c0392b22; padding: 8px; border-radius: 4px; }
 nav { margin: 12px 0; font-size: .9rem; }
</style></head><body>

{{if .Err}}<p class="alerte">Erreur : {{.Err}}</p>{{end}}

{{if .Channel}}
 <h1>Canal {{.Channel.ChannelID}}</h1>
 <p class="meta">Dernier relevé {{fmtTime .Channel.LastSeen}} —
   {{fmtValue .Channel.Power "dBmV"}} / {{fmtValue .Channel.SNR "dB"}}
   — fenêtre de {{.Hours}} h</p>
 <nav><a href="/">← tous les canaux</a> ·
   <a href="?heures=6">6 h</a> · <a href="?heures=24">24 h</a> ·
   <a href="?heures=168">7 j</a> · <a href="?heures=720">30 j</a></nav>
 <h2>Puissance (dBmV)</h2>
 {{.PowerSVG}}
 <h2>SNR (dB)</h2>
 {{.SNRSVG}}
{{else}}
 <h1>Relevés DOCSIS</h1>
 <p class="meta">Profil {{.Profile.Name}}{{if not .Profile.Calibrated}} — non calibré, l'état de la ligne n'est pas qualifié{{end}}
   · {{.Readings}} relevés · dernier {{fmtTime .Latest}}</p>
 <nav><a href="?heures=6">6 h</a> · <a href="?heures=24">24 h</a> ·
   <a href="?heures=168">7 j</a> · <a href="?heures=720">30 j</a></nav>
 {{if .Channels}}
 <table><tr><th>Canal</th><th>Modulation</th><th>Puissance</th><th>SNR</th><th>Vu à</th></tr>
 {{range .Channels}}
  <tr><td><a href="/canal/{{.ChannelID}}?heures={{$.Hours}}">{{.ChannelID}}</a></td>
   <td>{{.Modulation}}</td>
   <td>{{fmtValue .Power "dBmV"}}</td>
   <td>{{fmtValue .SNR "dB"}}</td>
   <td>{{fmtTime .LastSeen}}</td></tr>
 {{end}}</table>
 {{else}}<p>Aucun relevé sur la fenêtre choisie.</p>{{end}}
{{end}}
</body></html>`
