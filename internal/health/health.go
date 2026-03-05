// Package health provides liveness and readiness HTTP handlers for use with
// container orchestrators (Kubernetes, Docker Swarm, etc.) and load balancers.
package health

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadinessFunc is called by the /readyz handler to determine whether the
// service is ready to accept traffic. Return nil to signal readiness.
type ReadinessFunc func() error

// indexHTML is the dashboard served at GET /.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>go-smtp relay · status</title>
<style>
  :root {
    --bg:      #0f1117;
    --surface: #1a1d27;
    --border:  #2a2d3e;
    --text:    #e2e8f0;
    --muted:   #718096;
    --green:   #48bb78;
    --red:     #fc8181;
    --yellow:  #f6e05e;
    --blue:    #63b3ed;
    --accent:  #7c3aed;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg); color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    font-size: 15px; min-height: 100vh; padding: 2rem 1rem;
  }
  header { max-width: 860px; margin: 0 auto 2.5rem; display: flex; align-items: center; gap: 1rem; }
  .logo { width: 42px; height: 42px; background: var(--accent); border-radius: 10px;
    display: flex; align-items: center; justify-content: center; font-size: 1.4rem; flex-shrink: 0; }
  h1 { font-size: 1.4rem; font-weight: 700; }
  h1 span { color: var(--muted); font-weight: 400; font-size: 1rem; margin-left: .5rem; }
  .grid { max-width: 860px; margin: 0 auto;
    display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 1.25rem; }
  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px;
    padding: 1.25rem 1.5rem; display: flex; flex-direction: column; gap: .6rem;
    transition: border-color .15s; }
  .card:hover { border-color: var(--accent); }
  .card-header { display: flex; align-items: center; justify-content: space-between; }
  .card-title { font-weight: 600; font-size: .95rem; }
  .badge { display: inline-flex; align-items: center; gap: .35rem; padding: .2rem .65rem;
    border-radius: 9999px; font-size: .78rem; font-weight: 600;
    text-transform: uppercase; letter-spacing: .04em; }
  .badge.ok      { background: rgba(72,187,120,.15);  color: var(--green); }
  .badge.error   { background: rgba(252,129,129,.15); color: var(--red);   }
  .badge.pending { background: rgba(246,224,94,.12);  color: var(--yellow);}
  .badge.info    { background: rgba(99,179,237,.12);  color: var(--blue);  }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
  .badge.ok .dot    { animation: pulse-green 2s infinite; }
  .badge.error .dot { animation: pulse-red   2s infinite; }
  @keyframes pulse-green { 0%,100%{box-shadow:0 0 0 0 rgba(72,187,120,.6)} 50%{box-shadow:0 0 0 5px rgba(72,187,120,0)} }
  @keyframes pulse-red   { 0%,100%{box-shadow:0 0 0 0 rgba(252,129,129,.6)} 50%{box-shadow:0 0 0 5px rgba(252,129,129,0)} }
  .card-desc { color: var(--muted); font-size: .85rem; line-height: 1.5; }
  .card-link { margin-top: .25rem; display: inline-block; color: var(--blue);
    font-size: .83rem; text-decoration: none; font-family: "SF Mono","Fira Code",monospace; }
  .card-link:hover { text-decoration: underline; }
  .card-error { color: var(--red); font-size: .8rem; margin-top: .1rem; }
  footer { max-width: 860px; margin: 2.5rem auto 0; color: var(--muted); font-size: .8rem;
    display: flex; justify-content: space-between; align-items: center;
    border-top: 1px solid var(--border); padding-top: 1rem; }
  #last-update { font-family: "SF Mono","Fira Code",monospace; }
</style>
</head>
<body>
<header>
  <div class="logo">&#9993;</div>
  <div><h1>go-smtp relay <span>status dashboard</span></h1></div>
</header>
<div class="grid">
  <div class="card">
    <div class="card-header">
      <span class="card-title">Liveness</span>
      <span class="badge pending" id="badge-healthz"><span class="dot"></span>checking&#8230;</span>
    </div>
    <p class="card-desc">The process is alive and the HTTP server can accept connections.</p>
    <a class="card-link" href="/healthz" target="_blank">/healthz</a>
  </div>
  <div class="card">
    <div class="card-header">
      <span class="card-title">Readiness</span>
      <span class="badge pending" id="badge-readyz"><span class="dot"></span>checking&#8230;</span>
    </div>
    <p class="card-desc">The relay is ready to accept inbound SMTP connections.</p>
    <span id="readyz-error" class="card-error"></span>
    <a class="card-link" href="/readyz" target="_blank">/readyz</a>
  </div>
  <div class="card">
    <div class="card-header">
      <span class="card-title">Metrics</span>
      <span class="badge info"><span class="dot"></span>prometheus</span>
    </div>
    <p class="card-desc">Live Prometheus metrics dashboard: connections, auth, messages, Graph API, OAuth, webhooks.</p>
    <a class="card-link" href="/metrics">&#8594; Open dashboard</a>
  </div>
</div>
<footer>
  <span>Refreshes every 5 s</span>
  <span id="last-update">&#8212;</span>
</footer>
<script>
  function setBadge(id, ok, errText) {
    var el = document.getElementById(id);
    el.className = 'badge ' + (ok ? 'ok' : 'error');
    el.innerHTML = '<span class="dot"></span>' + (ok ? 'ok' : 'error');
    if (id === 'badge-readyz') document.getElementById('readyz-error').textContent = errText || '';
  }
  async function poll() {
    try { var r = await fetch('/healthz'); setBadge('badge-healthz', r.ok); }
    catch(e) { setBadge('badge-healthz', false); }
    try {
      var r2 = await fetch('/readyz');
      var body = await r2.json().catch(function(){ return {}; });
      setBadge('badge-readyz', r2.ok, body.error);
    } catch(e) { setBadge('badge-readyz', false, e.message); }
    document.getElementById('last-update').textContent = 'updated ' + new Date().toLocaleTimeString();
  }
  poll(); setInterval(poll, 5000);
</script>
</body>
</html>`

// metricsHTML is the interactive metrics dashboard served at GET /metrics (without ?$output=text).
// It fetches the raw Prometheus text from /metrics?$output=text, parses it client-side,
// and renders grouped, searchable metric family cards that auto-refresh every 15 s.
const metricsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>go-smtp relay · metrics</title>
<style>
  :root {
    --bg:#0f1117; --surface:#1a1d27; --surface2:#212438;
    --border:#2a2d3e; --text:#e2e8f0; --muted:#718096;
    --green:#48bb78; --red:#fc8181; --yellow:#f6e05e;
    --blue:#63b3ed; --purple:#b794f4; --orange:#f6ad55;
    --accent:#7c3aed;
  }
  *{box-sizing:border-box;margin:0;padding:0}
  body{background:var(--bg);color:var(--text);
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
    font-size:14px;min-height:100vh;padding:1.5rem 1rem}
  a{color:var(--blue);text-decoration:none}
  a:hover{text-decoration:underline}
  /* ---- header ---- */
  header{max-width:920px;margin:0 auto 1.25rem;
    display:flex;align-items:center;gap:.75rem;flex-wrap:wrap}
  .logo{width:34px;height:34px;background:var(--accent);border-radius:8px;
    display:flex;align-items:center;justify-content:center;font-size:1.1rem;flex-shrink:0}
  .header-title{flex:1;min-width:120px}
  h1{font-size:1.15rem;font-weight:700}
  h1 small{color:var(--muted);font-weight:400;font-size:.82rem;margin-left:.4rem}
  .controls{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap}
  #search{background:var(--surface);border:1px solid var(--border);color:var(--text);
    border-radius:7px;padding:.38rem .7rem;font-size:.83rem;width:190px;outline:none}
  #search:focus{border-color:var(--accent)}
  #search::placeholder{color:var(--muted)}
  .btn{background:var(--surface);border:1px solid var(--border);color:var(--text);
    border-radius:7px;padding:.38rem .8rem;font-size:.8rem;cursor:pointer;
    white-space:nowrap;transition:border-color .15s;text-decoration:none;
    display:inline-flex;align-items:center;gap:.3rem}
  .btn:hover{border-color:var(--accent);color:var(--blue);text-decoration:none}
  .btn-muted{color:var(--muted)}
  #countdown{font-size:.73rem;color:var(--muted);font-family:"SF Mono","Fira Code",monospace}
  /* ---- sections ---- */
  main{max-width:920px;margin:0 auto}
  section{margin-bottom:1.75rem}
  .sec-head{display:flex;align-items:center;gap:.5rem;margin-bottom:.6rem;
    cursor:pointer;user-select:none;padding:.2rem 0}
  .sec-head:hover h2{color:var(--text)}
  h2{font-size:.78rem;font-weight:700;text-transform:uppercase;
    letter-spacing:.07em;color:var(--muted)}
  .sec-count{font-size:.7rem;background:var(--surface);border:1px solid var(--border);
    border-radius:9999px;padding:.1rem .45rem;color:var(--muted)}
  .sec-arrow{font-size:.65rem;color:var(--muted);margin-left:auto}
  .sec-body{display:flex;flex-direction:column;gap:.4rem}
  .sec-body.collapsed{display:none}
  /* ---- metric family card ---- */
  .fam{background:var(--surface);border:1px solid var(--border);
    border-radius:9px;overflow:hidden;transition:border-color .12s}
  .fam:hover{border-color:#3a3d5e}
  .fam.hidden{display:none}
  .fam-head{display:flex;align-items:baseline;gap:.55rem;padding:.65rem 1rem;flex-wrap:wrap}
  .fam-name{font-family:"SF Mono","Fira Code",monospace;font-size:.8rem;font-weight:600}
  .type-pill{font-size:.68rem;font-weight:700;padding:.12rem .45rem;
    border-radius:9999px;text-transform:uppercase;letter-spacing:.05em;flex-shrink:0}
  .tp-counter  {background:rgba(99,179,237,.14); color:var(--blue)}
  .tp-gauge    {background:rgba(72,187,120,.14);  color:var(--green)}
  .tp-histogram{background:rgba(183,148,244,.14); color:var(--purple)}
  .tp-summary  {background:rgba(246,173,85,.14);  color:var(--orange)}
  .tp-unit     {background:rgba(255,255,255,.06);  color:var(--muted)}
  .fam-help{font-size:.78rem;color:var(--muted);flex:1;text-align:right;line-height:1.4}
  /* ---- sample rows ---- */
  .samples{border-top:1px solid var(--border)}
  .srow{display:flex;align-items:center;justify-content:space-between;gap:.6rem;
    padding:.38rem 1rem;border-bottom:1px solid rgba(42,45,62,.6);font-size:.8rem}
  .srow:last-child{border-bottom:none}
  .srow:nth-child(even){background:rgba(255,255,255,.012)}
  .slabels{display:flex;gap:.28rem;flex-wrap:wrap;flex:1}
  .lpill{background:var(--surface2);border:1px solid var(--border);border-radius:5px;
    padding:.08rem .38rem;font-family:"SF Mono","Fira Code",monospace;font-size:.72rem;color:var(--muted)}
  .lpill .lk{color:var(--blue)}
  .lpill .lv{color:var(--text);font-weight:500}
  .ssuf{color:var(--muted);font-size:.72rem;font-family:"SF Mono","Fira Code",monospace;flex-shrink:0}
  .sval-wrap{display:flex;align-items:baseline;gap:.25rem;flex-shrink:0;justify-content:flex-end;min-width:4.5rem}
  .sval{font-family:"SF Mono","Fira Code",monospace;font-weight:700;font-size:.88rem;text-align:right}
  .sunit{font-family:"SF Mono","Fira Code",monospace;font-size:.7rem;color:var(--muted);flex-shrink:0}
  .v-pos{color:var(--green)} .v-zero{color:var(--muted)} .v-inf{color:var(--yellow)}
  .bucket-note{padding:.3rem 1rem;font-size:.75rem;color:var(--muted);
    border-top:1px solid var(--border);
    background:rgba(255,255,255,.01)}
  .bucket-note a{color:var(--muted);font-family:"SF Mono","Fira Code",monospace}
  .bucket-note a:hover{color:var(--blue)}
  /* ---- status bar ---- */
  #status-bar{max-width:920px;margin:1.25rem auto 0;display:flex;
    justify-content:space-between;align-items:center;
    border-top:1px solid var(--border);padding-top:.65rem;
    font-size:.73rem;color:var(--muted);font-family:"SF Mono","Fira Code",monospace}
  #err{color:var(--red);padding:2rem;text-align:center;display:none;font-size:.9rem}
  .spin{display:inline-block;width:11px;height:11px;border:2px solid var(--border);
    border-top-color:var(--accent);border-radius:50%;animation:spin .65s linear infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body>
<header>
  <div class="logo">&#9993;</div>
  <div class="header-title">
    <h1>go-smtp relay <small>metrics dashboard</small></h1>
  </div>
  <div class="controls">
    <input id="search" type="search" placeholder="&#128269; filter metrics&#8230;"/>
    <button class="btn" id="rbtn" onclick="load()">&#8635; Refresh</button>
    <span id="countdown"></span>
    <a class="btn btn-muted" href="/metrics?$output=text" target="_blank">&#128196; Raw</a>
    <a class="btn btn-muted" href="/">&#8592; Status</a>
  </div>
</header>
<main>
  <div id="err"></div>
  <div id="content"></div>
</main>
<div id="status-bar">
  <span id="fam-count">&#8212;</span>
  <span id="ts">&#8212;</span>
</div>
<script>
var GROUPS=[
  {id:'smtp',    label:'SMTP',     px:['smtp_']},
  {id:'graph',   label:'Graph API',px:['graph_']},
  {id:'oauth',   label:'OAuth',    px:['oauth_']},
  {id:'webhook', label:'Webhooks', px:['webhook_']},
  {id:'runtime', label:'Runtime',  px:['go_','process_']},
  {id:'other',   label:'Other',    px:[]}
];

function classify(name){
  for(var i=0;i<GROUPS.length;i++){
    var g=GROUPS[i];
    if(!g.px.length) return g.id;
    for(var j=0;j<g.px.length;j++) if(name.indexOf(g.px[j])===0) return g.id;
  }
  return 'other';
}

function parsePrometheus(text){
  var fams=[],cur=null;
  var lines=text.split('\n');
  for(var i=0;i<lines.length;i++){
    var l=lines[i].trim();
    if(!l) continue;
    if(l.indexOf('# HELP ')===0){
      var rest=l.slice(7),sp=rest.indexOf(' ');
      cur={name:sp<0?rest:rest.slice(0,sp),help:sp<0?'':rest.slice(sp+1),type:'untyped',samples:[]};
      fams.push(cur);
    } else if(l.indexOf('# TYPE ')===0&&cur){
      var p=l.slice(7).trim().split(/\s+/);
      if(p.length>=2) cur.type=p[1];
    } else if(l[0]!=='#'&&cur){
      var bi=l.indexOf('{'),name2,labels='',value;
      if(bi>=0){
        name2=l.slice(0,bi);
        var be=l.lastIndexOf('}');
        labels=l.slice(bi+1,be);
        value=l.slice(be+1).trim().split(/\s+/)[0];
      } else {
        var pts=l.split(/\s+/);
        name2=pts[0]; value=pts[1]||'0';
      }
      cur.samples.push({name:name2,labels:labels,value:value});
    }
  }
  return fams;
}

function parseLbls(s){
  var res=[],re=/(\w+)="([^"]*)"/g,m;
  while((m=re.exec(s))!==null) res.push([m[1],m[2]]);
  return res;
}

function esc(s){
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// ---- Unit-aware formatting ----

// Infer the measurement domain from a metric family name.
function inferUnit(famName){
  if(/_bytes$|_byte$/.test(famName))   return 'bytes';
  if(/_seconds$|_second$/.test(famName)) return 'seconds';
  if(/_ratio$/.test(famName))          return 'ratio';
  return '';
}

// Format a plain count with K/M/B suffixes.
function fmtCount(n){
  if(!Number.isFinite(n)) return String(n);
  var abs=Math.abs(n);
  if(abs>=1e9) return (n/1e9).toPrecision(3)+'B';
  if(abs>=1e6) return (n/1e6).toPrecision(3)+'M';
  if(abs>=1e3) return (n/1e3).toPrecision(3)+'K';
  if(Number.isInteger(n)) return n.toLocaleString();
  return parseFloat(n.toPrecision(4)).toLocaleString(undefined,{maximumFractionDigits:4});
}

// Returns {val, unit} for a bytes value.
function fmtBytes(n){
  var abs=Math.abs(n);
  if(abs>=1073741824) return {val:(n/1073741824).toFixed(2),unit:'GB'};
  if(abs>=1048576)    return {val:(n/1048576).toFixed(2),   unit:'MB'};
  if(abs>=1024)       return {val:(n/1024).toFixed(2),      unit:'KB'};
  return {val:n.toFixed(0), unit:'B'};
}

// Returns {val, unit} for a seconds value.
function fmtDuration(n){
  var abs=Math.abs(n);
  if(abs>=3600)  return {val:(n/3600).toFixed(2),  unit:'h'};
  if(abs>=60)    return {val:(n/60).toFixed(2),     unit:'min'};
  if(abs>=1)     return {val:n.toFixed(3),          unit:'s'};
  if(abs>=0.001) return {val:(n*1000).toFixed(2),   unit:'ms'};
  return          {val:(n*1e6).toFixed(1),           unit:'µs'};
}

// Format a value given the family unit and the per-sample suffix (count, sum, bucket, etc.)
// Returns {val:string, unit:string}.
function fmtVal(rawVal, famUnit, suf){
  if(rawVal==='+Inf') return {val:'+Inf', unit:''};
  if(rawVal==='-Inf') return {val:'-Inf', unit:''};
  if(rawVal==='NaN')  return {val:'NaN',  unit:''};
  var n=parseFloat(rawVal);
  if(isNaN(n))        return {val:rawVal, unit:''};
  if(n===0)           return {val:'0',    unit:''};
  // _count and _total always mean a plain integer regardless of parent unit.
  if(suf==='count'||suf==='total') return {val:fmtCount(n), unit:''};
  switch(famUnit){
    case 'bytes':   return fmtBytes(n);
    case 'seconds': return fmtDuration(n);
    case 'ratio':   return {val:(n*100).toFixed(1), unit:'%'};
    default:        return {val:fmtCount(n), unit:''};
  }
}

function valCls(rawVal){
  if(rawVal==='+Inf'||rawVal==='-Inf') return 'v-inf';
  return parseFloat(rawVal)===0?'v-zero':'v-pos';
}


function getSuffix(famName,sampleName){
  if(sampleName===famName) return '';
  if(sampleName.indexOf(famName+'_')===0) return sampleName.slice(famName.length+1);
  return sampleName;
}

function displaySamples(fam){
  if(fam.type==='histogram'||fam.type==='summary'){
    return fam.samples.filter(function(s){
      var suf=getSuffix(fam.name,s.name);
      if(suf==='bucket') return false;
      if(fam.type==='summary'){
        var p=parseLbls(s.labels);
        if(p.length===1&&p[0][0]==='quantile') return false;
      }
      return true;
    });
  }
  return fam.samples;
}

function renderLabels(lblStr){
  var p=parseLbls(lblStr);
  if(!p.length) return '<span class="lpill"><span class="lv">&#8212;</span></span>';
  return p.map(function(kv){
    return '<span class="lpill"><span class="lk">'+esc(kv[0])+'</span>=<span class="lv">'+esc(kv[1])+'</span></span>';
  }).join('');
}

function renderFamily(fam){
  var ds=displaySamples(fam);
  var skipped=fam.samples.length-ds.length;
  var typeClass='tp-'+(fam.type||'untyped');
  var famUnit=inferUnit(fam.name);

  var rows=ds.map(function(s){
    var suf=getSuffix(fam.name,s.name);
    var fmt=fmtVal(s.value,famUnit,suf);
    return '<div class="srow">'+
      '<div class="slabels">'+renderLabels(s.labels)+'</div>'+
      (suf?'<span class="ssuf">'+esc(suf)+'</span>':'')+
      '<span class="sval-wrap">'+
        '<span class="sval '+valCls(s.value)+'">'+esc(fmt.val)+'</span>'+
        (fmt.unit?'<span class="sunit">'+esc(fmt.unit)+'</span>':'')+
      '</span>'+
    '</div>';
  }).join('');

  var note='';
  if(skipped>0){
    note='<div class="bucket-note">+'+skipped+' bucket'+(skipped!==1?'s':'')+
      ' hidden &mdash; <a href="/metrics?$output=text" target="_blank">view raw</a></div>';
  }

  return '<div class="fam" data-name="'+esc(fam.name)+'" data-help="'+esc((fam.help||'').toLowerCase())+'">'+
    '<div class="fam-head">'+
      '<span class="fam-name">'+esc(fam.name)+'</span>'+
      '<span class="type-pill '+typeClass+'">'+esc(fam.type)+'</span>'+
      (famUnit?'<span class="type-pill tp-unit">'+esc(famUnit)+'</span>':'')+
      (fam.help?'<span class="fam-help">'+esc(fam.help)+'</span>':'')+
    '</div>'+
    '<div class="samples">'+rows+note+'</div>'+
  '</div>';
}

// Snapshot the current collapsed state of every section before a DOM refresh.
function getSecState(){
  var s={};
  GROUPS.forEach(function(g){
    var el=document.getElementById('sbody-'+g.id);
    if(el) s[g.id]=el.classList.contains('collapsed');
  });
  return s;
}

function renderAll(fams,state){
  var grouped={};
  GROUPS.forEach(function(g){ grouped[g.id]=[]; });
  fams.forEach(function(f){ grouped[classify(f.name)].push(f); });

  var html='';
  GROUPS.forEach(function(g){
    var gf=grouped[g.id];
    if(!gf.length) return;
    // Use the saved state when available; fall back to default (runtime collapsed).
    var isCollapsed=(state&&g.id in state)?state[g.id]:(g.id==='runtime');
    var collapsed=isCollapsed?' collapsed':'';
    var arrow=isCollapsed?'&#9654;':'&#9660;';
    html+='<section>'+
      '<div class="sec-head" onclick="toggleSec(\''+g.id+'\')">'+
        '<h2>'+esc(g.label)+'</h2>'+
        '<span class="sec-count">'+gf.length+'</span>'+
        '<span class="sec-arrow" id="sarr-'+g.id+'">'+arrow+'</span>'+
      '</div>'+
      '<div class="sec-body'+collapsed+'" id="sbody-'+g.id+'">'+
        gf.map(renderFamily).join('')+
      '</div>'+
    '</section>';
  });
  return html;
}

function toggleSec(id){
  var body=document.getElementById('sbody-'+id);
  var arr=document.getElementById('sarr-'+id);
  var c=body.classList.toggle('collapsed');
  arr.innerHTML=c?'&#9654;':'&#9660;';
}

function applyFilter(q){
  q=q.toLowerCase().trim();
  document.querySelectorAll('.fam').forEach(function(el){
    var match=!q||el.dataset.name.indexOf(q)>=0||el.dataset.help.indexOf(q)>=0;
    el.classList.toggle('hidden',!match);
  });
  if(q){
    GROUPS.forEach(function(g){
      var body=document.getElementById('sbody-'+g.id);
      if(!body) return;
      if(body.querySelector('.fam:not(.hidden)')){
        body.classList.remove('collapsed');
        var arr=document.getElementById('sarr-'+g.id);
        if(arr) arr.innerHTML='&#9660;';
      }
    });
  }
}

document.getElementById('search').addEventListener('input',function(e){
  applyFilter(e.target.value);
});

var cdTimer=null,cdVal=15;
function startCountdown(){
  clearInterval(cdTimer);
  cdVal=15;
  updateCd();
  cdTimer=setInterval(function(){
    cdVal--;
    updateCd();
    if(cdVal<=0) load();
  },1000);
}
function updateCd(){
  var el=document.getElementById('countdown');
  if(el) el.textContent='next refresh in '+cdVal+'s';
}

async function load(){
  clearInterval(cdTimer);
  var savedState=getSecState(); // snapshot before DOM replacement
  var btn=document.getElementById('rbtn');
  if(btn) btn.innerHTML='<span class="spin"></span> Loading&#8230;';
  try{
    var resp=await fetch('/metrics?$output=text&_='+Date.now());
    if(!resp.ok) throw new Error('HTTP '+resp.status);
    var text=await resp.text();
    var fams=parsePrometheus(text);
    document.getElementById('content').innerHTML=renderAll(fams,savedState);
    document.getElementById('err').style.display='none';
    document.getElementById('fam-count').textContent=fams.length+' metric families';
    document.getElementById('ts').textContent='updated '+new Date().toLocaleTimeString();
    var q=document.getElementById('search').value;
    if(q) applyFilter(q);
  } catch(e){
    var em=document.getElementById('err');
    em.textContent='Failed to load metrics: '+e.message;
    em.style.display='block';
  } finally{
    if(btn) btn.innerHTML='&#8635; Refresh';
    startCountdown();
  }
}

load();
</script>
</body>
</html>`

// NewMux returns an http.ServeMux with the following routes pre-registered:
//
//	GET /                   — HTML status dashboard (liveness/readiness/metrics links)
//	GET /healthz            — liveness probe: always 200 while the process is running
//	GET /readyz             — readiness probe: 200 if readyFn returns nil, 503 otherwise
//	GET /metrics            — interactive HTML metrics dashboard
//	GET /metrics?$output=text — raw Prometheus text (for scrapers / CLI)
func NewMux(readyFn ReadinessFunc) *http.ServeMux {
	mux := http.NewServeMux()

	// Dashboard — HTML page with live status cards.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte(indexHTML)); err != nil {
			slog.Debug("index: failed to write response", "error", err)
		}
	})

	// Liveness — the process is alive as long as it can handle HTTP.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Debug("healthz: failed to write response", "error", err)
		}
	})

	// Readiness — delegates to the caller-supplied check function.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := readyFn(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if encErr := json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"error":  err.Error(),
			}); encErr != nil {
				slog.Debug("readyz: failed to write error response", "error", encErr)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ready"}); err != nil {
			slog.Debug("readyz: failed to write response", "error", err)
		}
	})

	// Metrics — HTML dashboard by default; raw Prometheus text when ?$output=text is set.
	// This allows Prometheus scrapers to be configured with metrics_path: /metrics?$output=text
	// while browsers get the interactive UI at /metrics.
	rawMetrics := promhttp.Handler()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("$output") == "text" {
			rawMetrics.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte(metricsHTML)); err != nil {
			slog.Debug("metrics: failed to write response", "error", err)
		}
	})

	return mux
}
