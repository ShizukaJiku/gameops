package gateway

import (
	"html/template"
	"net/http"

	"github.com/ShizukaJiku/gameops/internal/gamecontrol"
)

var loginTmpl = template.Must(template.New("login").Parse(`<!doctype html>
<html><head><title>gameops</title><style>
body{font-family:sans-serif;max-width:400px;margin:80px auto;background:#111;color:#eee}
input{width:100%;padding:8px;margin:8px 0;background:#222;color:#eee;border:1px solid #444;box-sizing:border-box}
button{width:100%;padding:8px;background:#2a6;color:#fff;border:0;cursor:pointer}
.error{color:#f66}
</style></head>
<body>
<h1>gameops</h1>
{{if .}}<p class="error">{{.}}</p>{{end}}
<form method="post" action="/login">
<input type="password" name="password" placeholder="Password" autofocus>
<button type="submit">Entrar</button>
</form>
</body></html>`))

func renderLogin(w http.ResponseWriter, status int, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	loginTmpl.Execute(w, errMsg)
}

type dashboardData struct {
	Instances []instanceRef
}

type instanceRef struct {
	Host string
	Name string
}

type fragmentData struct {
	Host   string
	Name   string
	Status gamecontrol.Status
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html><head><title>gameops</title><style>
body{font-family:sans-serif;max-width:800px;margin:40px auto;background:#111;color:#eee}
.card{border:1px solid #333;border-radius:8px;padding:16px;margin:12px 0}
button{padding:6px 12px;margin-right:8px;background:#333;color:#eee;border:1px solid #555;cursor:pointer}
button:hover{background:#444}
.online{color:#6f6} .offline{color:#f66}
</style></head>
<body>
<h1>gameops</h1>
{{range .Instances}}
<div class="card" id="instance-{{.Host}}-{{.Name}}" data-host="{{.Host}}" data-name="{{.Name}}">Cargando...</div>
{{end}}
<script>
function refresh(el) {
  var host = el.dataset.host, name = el.dataset.name;
  fetch('/hosts/' + host + '/instances/' + name + '/fragment')
    .then(function(r){ return r.text(); })
    .then(function(html){ el.innerHTML = html; });
}
function action(host, name, act) {
  fetch('/hosts/' + host + '/instances/' + name + '/' + act, {method: 'POST'})
    .then(function(r){ return r.text(); })
    .then(function(html){ document.getElementById('instance-' + host + '-' + name).innerHTML = html; });
}
var cards = document.querySelectorAll('.card');
cards.forEach(refresh);
setInterval(function() { cards.forEach(refresh); }, 5000);
</script>
</body></html>`))

var fragmentTmpl = template.Must(template.New("fragment").Parse(`<strong>{{.Name}}</strong> —
{{if .Status.Online}}<span class="online">online</span> — {{.Status.PlayerCount}}/{{.Status.MaxPlayers}} jugadores — uptime {{.Status.UptimeSec}}s{{else}}<span class="offline">offline</span>{{end}}
<div>
<button onclick="action('{{.Host}}','{{.Name}}','start')">Start</button>
<button onclick="action('{{.Host}}','{{.Name}}','stop')">Stop</button>
<button onclick="action('{{.Host}}','{{.Name}}','restart')">Restart</button>
</div>`))

func renderDashboard(w http.ResponseWriter, refs []instanceRef) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTmpl.Execute(w, dashboardData{Instances: refs})
}

func renderFragment(w http.ResponseWriter, host, name string, status gamecontrol.Status) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fragmentTmpl.Execute(w, fragmentData{Host: host, Name: name, Status: status})
}
