package gateway

import (
	"html/template"
	"net/http"
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
