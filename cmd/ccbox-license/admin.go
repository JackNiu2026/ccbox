package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/sylvinus/agent-vm/internal/license"
)

type adminView struct {
	Quantity int
	Max      int
	Codes    string
	Error    string
}

const adminJS = `const form=document.querySelector("form");
form.addEventListener("submit",()=>{form.querySelector("button").disabled=true;});
const copy=document.querySelector("#copy");
if(copy){copy.hidden=false;copy.addEventListener("click",async()=>{
  const codes=document.querySelector("#issued");
  const status=document.querySelector("#copy-status");
  try{await navigator.clipboard.writeText(codes.value);status.textContent="已复制全部激活码";}
  catch{codes.focus();codes.select();status.textContent="已选中全部激活码，请手动复制";}
});}`

var adminScriptHash = sha256.Sum256([]byte(adminJS))

var adminPage = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CCBox 激活码管理</title>
<style>
body{margin:0;background:#faf8f4;color:#292522;font:16px/1.65 system-ui,sans-serif}
header{padding:16px max(24px,calc((100% - 660px)/2));background:#1b1917;color:#fff7ee;font-weight:700}
main{max-width:660px;margin:clamp(32px,9vw,94px) auto;padding:0 24px}
h1{font-size:clamp(30px,6vw,46px);letter-spacing:-.02em;margin:0 0 12px}
p{margin:0 0 24px} button{padding:11px 18px;background:#ffa460;color:#23150d;border:0;border-radius:7px;font:inherit;font-weight:650;cursor:pointer}
button:focus-visible,input:focus-visible,textarea:focus-visible{outline:3px solid #963d13;outline-offset:4px}
button:disabled{opacity:.65;cursor:wait}
section{margin:24px 0;border-block:1px solid #ded8d0;padding:20px 0}
label{display:block;margin-bottom:8px;font-weight:650}
input,textarea{box-sizing:border-box;padding:12px;border:1px solid #a3978c;border-radius:6px;background:white;color:#292522;font:inherit}
input{width:112px} textarea{width:100%;resize:vertical;white-space:pre;font:clamp(12px,3.2vw,15px)/1.7 monospace}
.actions{display:flex;flex-wrap:wrap;align-items:center;gap:12px;margin:12px 0}
.hint{color:#61574e} [role=alert]{color:#963d13}
</style><header>CCBox</header><main><h1>激活码管理</h1>
<p>每个码可为一台设备激活或续期 30 天。</p>
<form method="post" action="/admin/license/"><label for="count">生成数量</label>
<div class="actions"><input type="number" id="count" name="count" min="1" max="{{.Max}}" step="1" value="{{.Quantity}}" required aria-describedby="count-hint"><button type="submit">生成激活码</button></div>
<p id="count-hint" class="hint">每批可生成 1 至 {{.Max}} 枚。</p></form>
{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
{{if .Codes}}<section><label for="issued">已生成 {{.Quantity}} 枚激活码</label><textarea id="issued" readonly rows="6" wrap="off">{{.Codes}}</textarea>
<div class="actions"><button id="copy" type="button" hidden>复制全部</button><span id="copy-status" role="status"></span></div>
<p class="hint">每行一枚。请在离开此页前复制，后台只保存哈希。</p></section>{{end}}
</main><script>` + adminJS + `</script></html>`))

func adminHandler(store *license.Store, password string) http.HandlerFunc {
	expected := sha256.Sum256([]byte(password))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'sha256-"+base64.StdEncoding.EncodeToString(adminScriptHash[:])+"'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		user, provided, ok := r.BasicAuth()
		digest := sha256.Sum256([]byte(provided))
		if !ok || user != "admin" || subtle.ConstantTimeCompare(digest[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="CCBox activation", charset="UTF-8"`)
			http.Error(w, "需要管理员凭据", http.StatusUnauthorized)
			return
		}
		view := adminView{Quantity: 1, Max: license.MaxIssueBatch}
		status := http.StatusOK
		if r.Method == http.MethodPost {
			origin := r.Header.Get("Origin")
			local := r.Host == "127.0.0.1:8791" && origin == "http://127.0.0.1:8791"
			site := r.Host == "ccbox.org" && origin == "https://ccbox.org"
			if !local && !site {
				http.Error(w, "请求来源无效", http.StatusForbidden)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
			values, parseErr := url.ParseQuery(string(body))
			count, supplied := values["count"]
			if parseErr == nil && supplied && len(count) == 1 {
				view.Quantity, parseErr = strconv.Atoi(count[0])
			}
			if err != nil || parseErr != nil || len(count) > 1 || view.Quantity < 1 || view.Quantity > view.Max {
				view.Quantity = 1
				view.Error = "请输入 1 至 " + strconv.Itoa(view.Max) + " 之间的整数。"
				status = http.StatusBadRequest
			} else {
				codes, err := store.IssueBatch(view.Quantity)
				if err != nil {
					view.Error = "暂时无法生成激活码，请稍后重试。"
					status = http.StatusServiceUnavailable
				} else {
					view.Codes = strings.Join(codes, "\n")
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_ = adminPage.Execute(w, view)
	}
}
