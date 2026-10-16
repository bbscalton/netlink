package main

const uiHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Netlink</title><style>
body{font-family:system-ui,sans-serif;max-width:760px;margin:20px auto;padding:0 12px;background:#111;color:#eee}
input[type=text]{width:100%;padding:8px;box-sizing:border-box;background:#222;color:#eee;border:1px solid #444}
button{padding:7px 14px;margin-top:8px;background:#2a6;color:#fff;border:0;cursor:pointer}
.job{border:1px solid #333;padding:10px;margin:10px 0}.bar{height:8px;background:#333}.bar div{height:8px;background:#2a6}
small{color:#999}a{color:#6cf}.x{background:#a33;float:right;margin:0}
</style></head><body><h2>Netlink <small>multi-WAN downloader</small></h2>
<input type="text" id="u" placeholder="https://example.com/file.iso">
<div id="ifs"></div><button onclick="go()">Download</button><div id="jobs"></div>
<script>
const mb=b=>(b/1e6).toFixed(1)+' MB';
fetch('/api/interfaces').then(r=>r.json()).then(l=>{ifs.innerHTML=l.map(i=>'<label><input type=checkbox checked value="'+i.name+'"> '+i.name+' <small>'+(i.device||'default route')+'</small></label> ').join('')});
function go(){const s=[...document.querySelectorAll('#ifs input:checked')].map(e=>e.value);
fetch('/api/jobs',{method:'POST',body:JSON.stringify({url:u.value.trim(),ifaces:s})}).then(()=>{u.value='';tick()})}
function del(id){fetch('/api/jobs/'+id,{method:'DELETE'}).then(tick)}
function esc(s){return String(s).replace(/[&<>"]/g,c=>'&#'+c.charCodeAt(0)+';')}
function tick(){fetch('/api/jobs').then(r=>r.json()).then(js=>{jobs.innerHTML=(js||[]).map(j=>{
const p=j.Total>0?100*j.Done/j.Total:0,sp=j.Seconds>0?mb(j.Done/j.Seconds)+'/s':'';
return '<div class=job><button class=x onclick="del(\''+j.ID+'\')">×</button><b>'+esc(j.File)+'</b> <small>'+j.Status+(j.Ranges?'':' (no range support: single stream)')+'</small>'+
'<div class=bar><div style="width:'+p.toFixed(1)+'%"></div></div><small>'+mb(j.Done)+' / '+mb(j.Total)+' · '+sp+' · '+
Object.entries(j.PerIface).map(([k,v])=>esc(k)+': '+mb(v)).join(', ')+'</small>'+(j.Err?'<br><small style="color:#f66">'+esc(j.Err)+'</small>':'')+
(j.Status=='done'?'<br><a href="/api/jobs/'+j.ID+'/file">Save to this device</a>':'')+'</div>'}).join('')})}
tick();setInterval(tick,1000);
</script></body></html>`
