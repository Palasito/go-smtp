package configgen

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Palasito/go-smtp/internal/config"
)

type schemaResponse struct {
	Groups []string          `json:"groups"`
	Fields []config.FieldDef `json:"fields"`
}

// RegisterRoutes adds the /generator and /generator/schema endpoints to the mux.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /generator", handleGeneratorPage)
	mux.HandleFunc("GET /generator/schema", handleSchema)
}

func handleSchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := schemaResponse{
		Groups: config.Groups(),
		Fields: config.Schema,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Debug("generator/schema: failed to write response", "error", err)
	}
}

func handleGeneratorPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(generatorHTML)); err != nil {
		slog.Debug("generator: failed to write response", "error", err)
	}
}

const generatorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>go-smtp relay &middot; config generator</title>
<style>
  :root {
    --bg:      #0f1117;
    --surface: #1a1d27;
    --surface2:#212438;
    --border:  #2a2d3e;
    --text:    #e2e8f0;
    --muted:   #718096;
    --green:   #48bb78;
    --red:     #fc8181;
    --yellow:  #f6e05e;
    --blue:    #63b3ed;
    --accent:  #7c3aed;
  }
  *{box-sizing:border-box;margin:0;padding:0}
  body{background:var(--bg);color:var(--text);
    font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
    font-size:15px;min-height:100vh;padding:2rem 1rem}

  header{max-width:920px;margin:0 auto 1.5rem;display:flex;align-items:center;gap:1rem}
  .logo{width:42px;height:42px;background:var(--accent);border-radius:10px;
    display:flex;align-items:center;justify-content:center;font-size:1.4rem;flex-shrink:0}
  h1{font-size:1.4rem;font-weight:700}
  h1 span{color:var(--muted);font-weight:400;font-size:1rem;margin-left:.5rem}

  .controls{max-width:920px;margin:0 auto 1.5rem;display:flex;align-items:center;
    gap:.6rem;flex-wrap:wrap;background:var(--surface);border:1px solid var(--border);
    border-radius:12px;padding:.75rem 1.25rem}
  .ctrl-select{background:var(--bg);border:1px solid var(--border);color:var(--text);
    border-radius:7px;padding:.4rem .7rem;font-size:.85rem;outline:none;cursor:pointer}
  .ctrl-select:focus{border-color:var(--accent)}
  .ctrl-btn{background:var(--surface2);border:1px solid var(--border);color:var(--text);
    border-radius:7px;padding:.4rem .9rem;font-size:.83rem;cursor:pointer;
    transition:border-color .15s;text-decoration:none;display:inline-flex;align-items:center;gap:.3rem}
  .ctrl-btn:hover{border-color:var(--accent);color:var(--blue);text-decoration:none}
  .ctrl-btn.primary{background:var(--accent);border-color:var(--accent);color:#fff;font-weight:600}
  .ctrl-btn.primary:hover{background:#6d28d9;color:#fff}
  .spacer{flex:1}
  .ctrl-sep{width:1px;height:1.4rem;background:var(--border)}

  .group{max-width:920px;margin:0 auto 1.25rem}
  .group-header{display:flex;align-items:center;gap:.6rem;cursor:pointer;
    user-select:none;padding:.5rem 0;border-bottom:1px solid var(--border);margin-bottom:.75rem}
  .group-header:hover h2{color:var(--text)}
  .group-arrow{font-size:.7rem;color:var(--muted);width:1rem;text-align:center}
  .group-header h2{font-size:.82rem;font-weight:700;text-transform:uppercase;
    letter-spacing:.06em;color:var(--muted)}
  .group-count{font-size:.72rem;background:var(--surface);border:1px solid var(--border);
    border-radius:9999px;padding:.1rem .5rem;color:var(--muted)}

  .field-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(380px,1fr));gap:.85rem}
  .field{background:var(--surface);border:1px solid var(--border);border-radius:10px;
    padding:1rem 1.15rem;transition:border-color .12s}
  .field:hover{border-color:#3a3d5e}
  .field.modified{border-left:3px solid var(--accent)}
  .field.has-error{border-left:3px solid var(--red)}
  .field.has-warn{border-left:3px solid var(--yellow)}
  .field-name{font-family:"SF Mono","Fira Code",monospace;font-weight:600;font-size:.85rem;display:block}
  .field-badges{display:inline-flex;gap:.3rem;margin-left:.4rem}
  .cond-badge{font-size:.68rem;background:rgba(246,224,94,.12);color:var(--yellow);
    padding:.1rem .4rem;border-radius:9999px;cursor:help}
  .field-desc{color:var(--muted);font-size:.8rem;margin:.3rem 0 .5rem;line-height:1.4}
  .input-row{display:flex;gap:.4rem;align-items:center}
  .field input,.field select{flex:1;background:var(--bg);border:1px solid var(--border);
    color:var(--text);border-radius:6px;padding:.45rem .65rem;font-size:.84rem;outline:none;
    font-family:inherit}
  .field input:focus,.field select:focus{border-color:var(--accent)}
  .field input::placeholder{color:var(--muted);font-size:.82rem}
  .toggle-vis{background:none;border:none;color:var(--muted);cursor:pointer;font-size:.9rem;
    padding:.2rem .3rem;border-radius:4px}
  .toggle-vis:hover{color:var(--text)}
  .field-hint{color:var(--muted);font-size:.72rem;margin-top:.3rem;
    font-family:"SF Mono","Fira Code",monospace}
  .field-error{font-size:.78rem;margin-top:.25rem;display:none;line-height:1.3}
  .field-error.visible{display:block}
  .field-error.err-sev{color:var(--red)}
  .field-error.warn-sev{color:var(--yellow)}

  .validation-summary{max-width:920px;margin:0 auto 1rem;padding:.8rem 1.1rem;
    border-radius:10px;display:none;font-size:.85rem;line-height:1.5}
  .validation-summary.visible{display:block}
  .validation-summary.vs-ok{background:rgba(72,187,120,.1);border:1px solid rgba(72,187,120,.3);color:var(--green)}
  .validation-summary.vs-err{background:rgba(252,129,129,.08);border:1px solid rgba(252,129,129,.25);color:var(--red)}

  .output-panel{max-width:920px;margin:2rem auto 0;display:none}
  .output-panel.visible{display:block}
  .output-header{display:flex;align-items:center;gap:.6rem;margin-bottom:.6rem;flex-wrap:wrap}
  .output-header h3{font-size:1rem;font-weight:600}
  .copy-ok{font-size:.8rem;color:var(--green);opacity:0;transition:opacity .3s}
  .copy-ok.flash{opacity:1}
  .output-code{background:var(--surface);border:1px solid var(--border);border-radius:10px;
    padding:1.25rem;font-family:"SF Mono","Fira Code",monospace;font-size:.8rem;
    white-space:pre;overflow-x:auto;max-height:520px;overflow-y:auto;line-height:1.55;
    color:var(--muted);tab-size:2}
  .output-code .oc-active{color:var(--text);font-weight:500}
  .output-code .oc-group{color:var(--blue)}

  footer{max-width:920px;margin:2rem auto 0;color:var(--muted);font-size:.8rem;
    display:flex;justify-content:space-between;align-items:center;
    border-top:1px solid var(--border);padding-top:.8rem}
  footer a{color:var(--blue);text-decoration:none;font-family:"SF Mono","Fira Code",monospace;font-size:.82rem}
  footer a:hover{text-decoration:underline}

  @media(max-width:480px){
    .field-grid{grid-template-columns:1fr}
  }
</style>
</head>
<body>
<header>
  <div class="logo">&#9881;</div>
  <div><h1>go-smtp relay <span>config generator</span></h1></div>
</header>

<div class="controls">
  <label style="font-size:.82rem;color:var(--muted)">Format:</label>
  <select id="format" class="ctrl-select" onchange="onFormatChange()">
    <option value="env">.env file</option>
    <option value="compose">docker-compose.yml</option>
  </select>
  <span id="cn-group" style="display:none;align-items:center;gap:.6rem">
    <span class="ctrl-sep"></span>
    <label style="font-size:.82rem;color:var(--muted)">Container name:</label>
    <input id="container-name" type="text" placeholder="smtp-relay" style="background:var(--bg);border:1px solid var(--border);color:var(--text);border-radius:7px;padding:.4rem .6rem;font-size:.84rem;width:140px;outline:none"/>
  </span>
  <div class="ctrl-sep"></div>
  <button class="ctrl-btn primary" onclick="generate()">&#9881; Generate</button>
  <button class="ctrl-btn" onclick="validateAll()">&#10003; Validate</button>
  <button class="ctrl-btn" onclick="resetAll()">&#8634; Reset</button>
  <span class="spacer"></span>
  <a class="ctrl-btn" href="/" style="color:var(--muted)">&#8592; Status</a>
</div>

<div class="validation-summary" id="validation-summary"></div>
<main id="form-container" style="max-width:920px;margin:0 auto">
  <p style="color:var(--muted);text-align:center;padding:2rem">Loading schema&#8230;</p>
</main>

<div class="output-panel" id="output-panel">
  <div class="output-header">
    <h3>Generated Configuration</h3>
    <button class="ctrl-btn" onclick="copyOutput()">&#128203; Copy</button>
    <button class="ctrl-btn" onclick="downloadOutput()">&#128190; Download</button>
    <span class="copy-ok" id="copy-ok">Copied!</span>
  </div>
  <div class="output-code" id="output-code"></div>
</div>

<footer>
  <span id="footer-info">&#8212;</span>
  <a href="/">&#8592; Back to status dashboard</a>
</footer>

<script>
var schema=[];
var groups=[];

function esc(s){
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function onFormatChange(){
  var isCompose=document.getElementById('format').value==='compose';
  document.getElementById('cn-group').style.display=isCompose?'inline-flex':'none';
}

async function init(){
  try{
    var resp=await fetch('/generator/schema');
    var data=await resp.json();
    schema=data.fields||[];
    groups=data.groups||[];
    renderForm();
    document.getElementById('footer-info').textContent=
      schema.length+' fields across '+groups.length+' groups';
  }catch(e){
    document.getElementById('form-container').innerHTML=
      '<p style="color:var(--red);text-align:center;padding:2rem">'+
      'Failed to load schema: '+esc(e.message)+'</p>';
  }
}

function fieldsByGroup(g){
  return schema.filter(function(f){return f.group===g;});
}

function renderForm(){
  var html='';
  for(var gi=0;gi<groups.length;gi++){
    var g=groups[gi];
    var fields=fieldsByGroup(g);
    html+='<div class="group">';
    html+='<div class="group-header" onclick="toggleGroup('+gi+')">';
    html+='<span class="group-arrow" id="garr-'+gi+'">&#9660;</span>';
    html+='<h2>'+esc(g)+'</h2>';
    html+='<span class="group-count">'+fields.length+'</span>';
    html+='</div>';
    html+='<div class="field-grid" id="gbody-'+gi+'">';
    for(var fi=0;fi<fields.length;fi++){
      html+=renderField(fields[fi]);
    }
    html+='</div></div>';
  }
  document.getElementById('form-container').innerHTML=html;
}

function renderField(f){
  var id='f-'+f.envVar;
  var h='<div class="field" id="field-'+f.envVar+'">';
  h+='<span class="field-name">'+esc(f.envVar);
  if(f.conditionallyRequired){
    h+='<span class="field-badges"><span class="cond-badge" title="'+esc(f.conditionallyRequired)+'">conditional</span></span>';
  }
  h+='</span>';
  h+='<p class="field-desc">'+esc(f.description)+'</p>';

  h+='<div class="input-row">';
  if(f.validValues&&f.validValues.length>0){
    h+='<select id="'+id+'" onchange="onFieldChange(\''+f.envVar+'\')">';
    h+='<option value="">default ('+esc(f["default"])+')</option>';
    for(var i=0;i<f.validValues.length;i++){
      h+='<option value="'+esc(f.validValues[i])+'">'+esc(f.validValues[i])+'</option>';
    }
    h+='</select>';
  }else if(f.type==='bool'){
    h+='<select id="'+id+'" onchange="onFieldChange(\''+f.envVar+'\')">';
    h+='<option value="">default ('+esc(f["default"])+')</option>';
    h+='<option value="true">true</option>';
    h+='<option value="false">false</option>';
    h+='</select>';
  }else{
    var it=f.sensitive?'password':'text';
    h+='<input type="'+it+'" id="'+id+'" placeholder="'+esc(f["default"])+'"';
    h+=' oninput="onFieldChange(\''+f.envVar+'\')"/>';
    if(f.sensitive){
      h+='<button class="toggle-vis" onclick="toggleVis(\''+id+'\')" title="Show/hide">&#128065;</button>';
    }
  }
  h+='</div>';

  if(f.validationHint){
    h+='<div class="field-hint">'+esc(f.validationHint)+'</div>';
  }
  h+='<div class="field-error" id="err-'+f.envVar+'"></div>';
  h+='</div>';
  return h;
}

function toggleGroup(gi){
  var body=document.getElementById('gbody-'+gi);
  var arr=document.getElementById('garr-'+gi);
  if(body.style.display==='none'){
    body.style.display='';
    arr.innerHTML='&#9660;';
  }else{
    body.style.display='none';
    arr.innerHTML='&#9654;';
  }
}

function toggleVis(inputId){
  var el=document.getElementById(inputId);
  el.type=el.type==='password'?'text':'password';
}

function onFieldChange(envVar){
  var el=document.getElementById('field-'+envVar);
  var input=document.getElementById('f-'+envVar);
  if(input.value){
    el.classList.add('modified');
  }else{
    el.classList.remove('modified');
  }
  clearFieldError(envVar);
}

function collectValues(){
  var values={};
  for(var i=0;i<schema.length;i++){
    var input=document.getElementById('f-'+schema[i].envVar);
    if(input&&input.value){
      values[schema[i].envVar]=input.value;
    }
  }
  return values;
}

function resetAll(){
  for(var i=0;i<schema.length;i++){
    var input=document.getElementById('f-'+schema[i].envVar);
    if(input){input.value='';input.type=schema[i].sensitive?'password':'text';}
    var el=document.getElementById('field-'+schema[i].envVar);
    if(el){el.classList.remove('modified','has-error','has-warn');}
    clearFieldError(schema[i].envVar);
  }
  document.getElementById('output-panel').classList.remove('visible');
  hideSummary();
}

/* ---- Generation ---- */

function generate(){
  var format=document.getElementById('format').value;
  var values=collectValues();
  var out=format==='compose'?generateCompose(values):generateEnv(values);
  document.getElementById('output-code').innerHTML=out;
  document.getElementById('output-panel').classList.add('visible');
  document.getElementById('output-panel').scrollIntoView({behavior:'smooth'});
}

function generateEnv(values){
  var l=[];
  l.push(esc('# ============================================================'));
  l.push(esc('# smtp-relay configuration'));
  l.push(esc('# Generated by: smtp-relay web generator'));
  l.push(esc('# ============================================================'));
  for(var gi=0;gi<groups.length;gi++){
    var g=groups[gi];
    var fields=fieldsByGroup(g);
    l.push('');
    l.push('<span class="oc-group">'+esc('# === '+g+' ===')+'</span>');
    l.push('');
    for(var fi=0;fi<fields.length;fi++){
      var f=fields[fi];
      l.push(esc('# '+f.description));
      if(f.validValues&&f.validValues.length>0){
        l.push(esc('# Valid values: '+f.validValues.join(', ')));
      }else if(f.validationHint){
        l.push(esc('# Hint: '+f.validationHint));
      }
      if(f.conditionallyRequired){
        l.push(esc('# Note: '+f.conditionallyRequired));
      }
      var val=values[f.envVar];
      if(val!==undefined){
        l.push('<span class="oc-active">'+esc(f.envVar+'='+val)+'</span>');
      }else{
        var def=f.sensitive?'CHANGE_ME':f["default"];
        l.push(esc('# '+f.envVar+'='+def));
      }
      l.push('');
    }
  }
  return l.join('\n');
}

function generateCompose(values){
  var smtpPort=values['SMTP_PORT']||'8025';
  var healthPort=values['HEALTH_PORT']||'9090';
  var cn=document.getElementById('container-name').value.trim()||'smtp-relay';
  var l=[];
  l.push(esc('# ============================================================'));
  l.push(esc('# smtp-relay docker-compose configuration'));
  l.push(esc('# Generated by: smtp-relay web generator'));
  l.push(esc('# ============================================================'));
  l.push('');
  l.push('<span class="oc-active">'+esc('services:')+'</span>');
  l.push('<span class="oc-active">'+esc('  smtp-relay:')+'</span>');
  l.push('<span class="oc-active">'+esc('    image: ghcr.io/palasito/go-smtp:latest')+'</span>');
  l.push('<span class="oc-active">'+esc('    container_name: '+cn)+'</span>');
  l.push(esc('    # Must be >= SHUTDOWN_TIMEOUT so Docker does not SIGKILL before the drain completes.'));
  l.push('<span class="oc-active">'+esc('    stop_grace_period: 35s')+'</span>');
  l.push('<span class="oc-active">'+esc('    restart: unless-stopped')+'</span>');
  l.push('<span class="oc-active">'+esc('    ports:')+'</span>');
  l.push('<span class="oc-active">'+esc('      - "'+smtpPort+':'+smtpPort+'"   # SMTP')+'</span>');
  l.push('<span class="oc-active">'+esc('      - "'+healthPort+':'+healthPort+'"   # Health/Metrics')+'</span>');
  l.push('<span class="oc-active">'+esc('    volumes:')+'</span>');
  l.push('<span class="oc-active">'+esc('      - ./certs:/certs:ro')+'</span>');
  l.push('<span class="oc-active">'+esc('      - ./logs:/logs')+'</span>');
  l.push('<span class="oc-active">'+esc('    environment:')+'</span>');
  for(var gi=0;gi<groups.length;gi++){
    var g=groups[gi];
    var fields=fieldsByGroup(g);
    l.push('      <span class="oc-group">'+esc('# === '+g+' ===')+'</span>');
    l.push('');
    for(var fi=0;fi<fields.length;fi++){
      var f=fields[fi];
      l.push(esc('      # '+f.description));
      if(f.validValues&&f.validValues.length>0){
        l.push(esc('      # Valid values: '+f.validValues.join(', ')));
      }else if(f.validationHint){
        l.push(esc('      # Hint: '+f.validationHint));
      }
      if(f.conditionallyRequired){
        l.push(esc('      # Note: '+f.conditionallyRequired));
      }
      var val=values[f.envVar];
      if(val!==undefined){
        l.push('<span class="oc-active">'+esc('      '+f.envVar+': "'+val+'"')+'</span>');
      }else{
        var def=f.sensitive?'CHANGE_ME':f["default"];
        l.push(esc('      # '+f.envVar+': "'+def+'"'));
      }
      l.push('');
    }
  }
  return l.join('\n');
}

/* ---- Validation ---- */

function clearFieldError(envVar){
  var el=document.getElementById('err-'+envVar);
  if(el){el.textContent='';el.className='field-error';}
}

function showFieldError(envVar,msg,sev){
  var el=document.getElementById('err-'+envVar);
  if(el){el.textContent=msg;el.className='field-error visible '+(sev==='warn'?'warn-sev':'err-sev');}
  var fld=document.getElementById('field-'+envVar);
  if(fld){fld.classList.add(sev==='warn'?'has-warn':'has-error');}
}

function hideSummary(){
  var el=document.getElementById('validation-summary');
  el.className='validation-summary';
  el.textContent='';
}

function showSummary(errors,warnings){
  var el=document.getElementById('validation-summary');
  if(errors===0&&warnings===0){
    el.className='validation-summary visible vs-ok';
    el.textContent='All values look good!';
  }else{
    el.className='validation-summary visible vs-err';
    el.textContent=errors+' error(s), '+warnings+' warning(s) found.';
  }
}

function findField(envVar){
  for(var i=0;i<schema.length;i++){
    if(schema[i].envVar===envVar) return schema[i];
  }
  return null;
}

function validateAll(){
  var values=collectValues();
  var errors=0,warnings=0;

  // Clear previous
  for(var i=0;i<schema.length;i++){
    clearFieldError(schema[i].envVar);
    var fld=document.getElementById('field-'+schema[i].envVar);
    if(fld){fld.classList.remove('has-error','has-warn');}
  }
  hideSummary();

  // Per-field
  for(var i=0;i<schema.length;i++){
    var f=schema[i];
    var val=values[f.envVar];
    if(!val) continue;

    if((f.type==='int'||f.type==='int64')&&!/^-?\d+$/.test(val)){
      showFieldError(f.envVar,'"'+val+'" is not a valid integer','err');
      errors++;continue;
    }
    if(f.type==='bool'&&val!=='true'&&val!=='false'){
      showFieldError(f.envVar,'must be "true" or "false"','err');
      errors++;continue;
    }
    if(f.validValues&&f.validValues.length>0){
      var ok=false;
      for(var vi=0;vi<f.validValues.length;vi++){
        if(f.validValues[vi].toLowerCase()===val.toLowerCase()){ok=true;break;}
      }
      if(!ok){
        showFieldError(f.envVar,'"'+val+'" must be one of: '+f.validValues.join(', '),'err');
        errors++;continue;
      }
    }
    if(f.validationHint&&f.validationHint.indexOf('port')>=0&&f.type==='string'){
      var p=parseInt(val,10);
      if(!isNaN(p)&&(p<1||p>65535)){
        showFieldError(f.envVar,'port must be 1-65535','err');
        errors++;
      }
    }
    if(f.envVar==='FAILURE_WEBHOOK_URL'&&val){
      if(val.indexOf('http://')!==0&&val.indexOf('https://')!==0){
        showFieldError(f.envVar,'must start with http:// or https://','err');
        errors++;
      }
    }
    if(f.validationHint&&f.validationHint.indexOf('positive')>=0&&(f.type==='int'||f.type==='int64')){
      var n=parseInt(val,10);
      if(!isNaN(n)&&n<=0){
        showFieldError(f.envVar,'must be a positive integer','err');
        errors++;
      }
    }
    if(f.validationHint&&f.validationHint.indexOf('non-negative')>=0&&(f.type==='int'||f.type==='int64')){
      var n=parseInt(val,10);
      if(!isNaN(n)&&n<0){
        showFieldError(f.envVar,'must be a non-negative integer','err');
        errors++;
      }
    }
  }

  // Cross-field: port collision
  if(values['SMTP_PORT']&&values['HEALTH_PORT']&&values['SMTP_PORT']===values['HEALTH_PORT']){
    showFieldError('HEALTH_PORT','same as SMTP_PORT','warn');
    warnings++;
  }

  // Cross-field: whitelist deps
  if(values['WHITELIST_IPS']){
    var deps=['WHITELIST_TENANT_ID','WHITELIST_CLIENT_ID','WHITELIST_CLIENT_SECRET'];
    for(var di=0;di<deps.length;di++){
      if(!values[deps[di]]){
        showFieldError(deps[di],'required when WHITELIST_IPS is set','err');
        errors++;
      }
    }
  }

  showSummary(errors,warnings);

  // Scroll to first error
  var firstErr=document.querySelector('.field.has-error,.field.has-warn');
  if(firstErr) firstErr.scrollIntoView({behavior:'smooth',block:'center'});
}

/* ---- Copy / Download ---- */

function getPlainOutput(){
  var el=document.getElementById('output-code');
  return el.textContent||el.innerText;
}

function copyOutput(){
  navigator.clipboard.writeText(getPlainOutput()).then(function(){
    var ok=document.getElementById('copy-ok');
    ok.classList.add('flash');
    setTimeout(function(){ok.classList.remove('flash');},1500);
  });
}

function downloadOutput(){
  var text=getPlainOutput();
  var format=document.getElementById('format').value;
  var fname=format==='compose'?'docker-compose.yml':'.env';
  var blob=new Blob([text],{type:'text/plain'});
  var a=document.createElement('a');
  a.href=URL.createObjectURL(blob);
  a.download=fname;
  a.click();
  URL.revokeObjectURL(a.href);
}

init();
</script>
</body>
</html>`
