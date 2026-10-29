"""Builds docs/index.html (browser version) from ui.html (desktop interface)."""
import sys
src, dst = sys.argv[1], sys.argv[2]
s = open(src).read()
def rep(a, b):
    global s
    assert a in s, 'missing: ' + a[:70]
    s = s.replace(a, b, 1)

rep('<div class="row" style="flex:0 0 auto"><span class="file" id="fileLabel">No file loaded</span><button id="quit" title="Stop the program">Quit</button></div>',
    '<div class="row" style="flex:0 0 auto"><span class="file" id="fileLabel">No file loaded</span><span class="local">Runs in your browser · your data never leaves your computer</span></div>')
rep('.busy{opacity:.55;pointer-events:none}', '''.busy{opacity:.55;pointer-events:none}
.local{font-size:12px;color:#2e8a66;background:#eaf7f1;border-radius:20px;padding:3px 10px}
#boot{position:fixed;inset:0;background:var(--bg);display:flex;align-items:center;justify-content:center;flex-direction:column;gap:10px;z-index:20;color:var(--muted)}
.spin{width:28px;height:28px;border:3px solid var(--line);border-top-color:var(--accent);border-radius:50%;animation:sp 0.9s linear infinite}
@keyframes sp{to{transform:rotate(360deg)}}
#status{position:fixed;bottom:16px;right:16px;background:#1d2330;color:#fff;padding:8px 14px;border-radius:8px;font-size:13px;display:none;z-index:15}
footer{padding:6px 22px 26px;color:var(--muted);font-size:12px;max-width:1100px}
footer a{color:var(--accent)}
.cite{background:#eef3fc;border:1px solid #d5e1f5;border-radius:10px;padding:10px 14px;margin-bottom:14px;font-size:13px;display:flex;gap:12px;align-items:flex-start;justify-content:space-between}
.cite p{margin:0}
.cite .ref{margin-top:3px;color:#334}
.cite button{flex:0 0 auto}''')
rep('<div class="empty" id="placeholder">', '<div class="cite" id="citeTop"></div>\n  <div class="empty" id="placeholder">')
rep('<div class="overlay" id="overlay"><img id="big" alt=""></div>', '''<div class="overlay" id="overlay"><img id="big" alt=""></div>
<div id="status"></div>
<div id="boot"><div class="spin"></div><div id="bootMsg">Loading Chord Diagram Studio…</div></div>
<footer>
  <p id="citeFoot"></p>
  <p><b>Privacy:</b> files are processed locally in your browser with WebAssembly; nothing is uploaded. A desktop version for Mac, Windows and Linux is available from the <a href="https://github.com/YOUR-USERNAME/YOUR-REPOSITORY/releases" id="relLink">project's GitHub releases</a>.</p>
  <p><b>AI disclosure:</b> The source code of Chord Diagram Studio was generated with Claude (Anthropic), an AI assistant, based on the author's specifications. The author defined the requirements and design, and tested and validated the program's output against independent calculations. The program draws figures directly from the input data; it does not use AI to create or modify them.</p>
</footer>
<script src="wasm_exec.js"></script>''')
rep("const PASTEL=[", """// ===== Citation: after Zenodo gives you the DOI, change only the next two lines =====
const CITE_DOI='10.5281/zenodo.23602942';
const CITE_VERSION='1.1.0';
// ====================================================================================
const CITE_TEXT=`Carretero-Hernandez M. Chord Diagram Studio (version ${CITE_VERSION}) [Software]. Zenodo. https://doi.org/${CITE_DOI}`;
function citeBox(){return `<div><p><b>Using these diagrams in a publication or presentation? Please cite Chord Diagram Studio:</b></p><p class="ref">${CITE_TEXT.replace(/(https:\\/\\/doi\\.org\\/\\S+)/,'<a href="$1" target="_blank" rel="noopener">$1</a>')}</p></div><button class="small" data-copycite>Copy citation</button>`}
function wireCite(root){root.querySelectorAll('[data-copycite]').forEach(b=>b.onclick=async()=>{try{await navigator.clipboard.writeText(CITE_TEXT);b.textContent='Copied ✓';setTimeout(()=>b.textContent='Copy citation',1500)}catch(e){prompt('Copy the citation:',CITE_TEXT)}})}
const PASTEL=[""")
rep("async function api(url,opts){const r=await fetch(url,opts);const j=await r.json();if(!r.ok)throw new Error(j.error||'Error');return j}",
"""const tick=()=>new Promise(r=>setTimeout(r,30));
function status(m){const s=$('status');s.textContent=m||'';s.style.display=m?'block':'none'}
function parse(x){const j=JSON.parse(x);if(j.error)throw new Error(j.error);return j}
let fileBytes=null,fileName='';
async function api(url,opts){
  await tick();
  if(url==='/api/load'){const fd=opts.body;const f=fd.get('file');
    if(f){fileBytes=new Uint8Array(await f.arrayBuffer());fileName=f.name;return parse(chordLoad(fileBytes,fileName,'',''))}
    return parse(chordLoad(null,'',fd.get('rsheet'),fd.get('psheet')))}
  if(url==='/api/categories')return parse(chordCategories(opts.body));
  if(url==='/api/generate')return parse(chordGenerate(opts.body));
  throw new Error('unknown '+url)}
function blobURL(i,kind){const b=chordFile(i,kind);if(!b)return null;
  const t={png:'image/png',svg:'image/svg+xml',xlsx:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',summary:'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'}[kind];
  return URL.createObjectURL(new Blob([b],{type:t}))}
function save(url,name){const a=document.createElement('a');a.href=url;a.download=name;document.body.appendChild(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),4000)}
let current=null;
async function download(i,kind){
  if(kind==='summary'){const u=blobURL(0,'summary');if(u)save(u,current.prefix+'_summary.xlsx');return}
  const d=current.diagrams[i];
  if(kind==='png'){status('Rendering 300 dpi PNG for '+d.name+'…');await tick()}
  try{const u=blobURL(i,kind);if(u)save(u,d.file+(kind==='xlsx'?'_counts.xlsx':'.'+kind))}finally{status('')}
}
async function downloadAll(){
  const n=current.diagrams.length;
  for(let i=0;i<n;i++){if(current.diagrams[i].error)continue;status(`Rendering PNG ${i+1} of ${n}…`);await tick();chordFile(i,'png')}
  status('Packing zip…');await tick();
  const b=chordZip();const pre=current.prefix+(current.diagrams.length>1?'_series':'');
  save(URL.createObjectURL(new Blob([b],{type:'application/zip'})),pre+'_all.zip');status('')
}""")
# results: links become JS handlers
rep('<a href="/api/zip?job=${j.job}">Download all (zip)</a>', '<a href="#" data-all="1">Download all (zip)</a>')
rep('<a href="/api/file?job=${j.job}&kind=summary">Summary (xlsx)</a>', '<a href="#" data-dl="0" data-k="summary">Summary (xlsx)</a>')
rep("const c=d.counts;const base=`/api/file?job=${j.job}&i=${i}`;", "const c=d.counts;const prev=d.error?null:blobURL(i,'svg');")
rep('${d.error?\'\':`<a href="${base}&kind=png&dl=1">PNG</a><a href="${base}&kind=svg&dl=1">SVG</a>`}<a href="${base}&kind=xlsx&dl=1">Counts (xlsx)</a>',
    '${d.error?\'\':`<a href="#" data-dl="${i}" data-k="png">PNG</a><a href="#" data-dl="${i}" data-k="svg">SVG</a>`}<a href="#" data-dl="${i}" data-k="xlsx">Counts (xlsx)</a>')
rep('`<img src="${base}&kind=png" alt="Chord diagram ${esc(d.name)}" data-big="${base}&kind=png">`', '`<img src="${prev}" alt="Chord diagram ${esc(d.name)}" data-big="${prev}">`')
rep("current=j;", "current=j;")  # marker must exist in ui.html
rep("let h=`<div class=\"bar\">", "let h=`<div class=\"cite\">${citeBox()}</div><div class=\"bar\">")
rep("  R.innerHTML=h;\n", "  R.innerHTML=h;\n  wireCite(R);\n")
rep("R.querySelectorAll('img[data-big]').forEach(im=>im.onclick=()=>{$('big').src=im.dataset.big;$('overlay').style.display='flex'});",
"""R.querySelectorAll('img[data-big]').forEach(im=>im.onclick=()=>{$('big').src=im.dataset.big;$('overlay').style.display='flex'});
  R.querySelectorAll('[data-dl]').forEach(a=>a.onclick=e=>{e.preventDefault();download(+a.dataset.dl,a.dataset.k)});
  R.querySelectorAll('[data-all]').forEach(a=>a.onclick=e=>{e.preventDefault();downloadAll()});""")
rep("$('quit').onclick=async()=>{try{await fetch('/api/quit',{method:'POST'})}catch(e){} document.body.innerHTML='<div class=\"empty\">Chord Diagram Studio has stopped. You can close this tab.</div>'};",
"""window.chordReady=()=>{$('boot').style.display='none'};
$('citeTop').innerHTML=citeBox();wireCite($('citeTop'));
$('citeFoot').innerHTML='<b>How to cite:</b> '+esc(CITE_TEXT);
(async()=>{try{const go=new Go();let r;
  if(WebAssembly.instantiateStreaming){try{r=await WebAssembly.instantiateStreaming(fetch('chordstudio.wasm'),go.importObject)}catch(e){r=null}}
  if(!r){const b=await (await fetch('chordstudio.wasm')).arrayBuffer();r=await WebAssembly.instantiate(b,go.importObject)}
  go.run(r.instance)}catch(e){$('bootMsg').textContent='Could not start: '+e.message+'. Open this page through a web server (e.g. GitHub Pages), not by double-clicking the file.'}})();""")
rep("let current=null;\n$('overlay').onclick", "$('overlay').onclick")
rep('<title>Chord Diagram Studio</title>', '<title>Chord Diagram Studio</title>\n<meta name="description" content="Make publication-ready chord diagrams of significant Spearman correlations between variable categories, directly in your browser.">')
open(dst, 'w').write(s)
print('ok')
