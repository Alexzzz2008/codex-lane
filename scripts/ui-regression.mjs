const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
const outputDir = process.env.QA_OUTPUT || '/tmp/codexlane-qa';
await mkdir(outputDir, { recursive: true });

const mock = `
const settings = {promptCodexProxy:false,protocol:'http',host:'127.0.0.1',port:7897,noProxy:'localhost,127.0.0.1,::1',engine:'auto',wslDistro:'',openCodex:false,stopCodexOnExit:false,retentionDays:14,maxMemoryRows:10000,upstreams:[],activeUpstream:'',autoFailover:false,hostFilterMode:'off',allowedHosts:[],routes:[],minimizeToTray:true,launchAtLogin:false};
const now = new Date().toISOString();
let state = {version:'0.1.39',repositoryUrl:'https://github.com/Alexzzz2008/codex-lane',releasesUrl:'https://github.com/Alexzzz2008/codex-lane/releases',settings,install:{managed:true},lane:{running:true,interfacePort:53999,kernelPort:54000,interfaceUrl:'http://127.0.0.1:53999',kernelUrl:'http://127.0.0.1:54000',startedAt:now},upstream:{reachable:true,url:'http://127.0.0.1:7897',latencyMs:88,checkedAt:now,domains:[]},codex:{running:false,installed:true,version:'1.0',proxyMatched:false,processes:[]},engine:{resolved:'windows'},wsl:{distros:[],interopOk:false},environment:{arch:'amd64',osName:'Windows',configPath:'C:/QA/launcher.json'},dependencies:[],stats:{total:2,success:1,failed:1,successRate:50,bytesUp:32,bytesDown:2048,p95Ms:44},requests:[{id:'pending',time:'09:30:01.123',host:'pending.test',method:'CONNECT',source:'kernel',port:443,scheme:'https',result:'',status:0},{id:'failed',time:'09:30:00.123',host:'failed.test',method:'CONNECT',source:'interface',port:443,scheme:'https',result:'failed',status:502}],series:Array.from({length:60},(_,index)=>({time:'09:'+String(index).padStart(2,'0'),bytesDown:index*128,latencyMs:30+index})),events:[],update:{status:'current',currentVersion:'0.1.39',available:false}};
window.__qa={saves:[],saveDelay:600,history:[],get state(){return state;}};
const clone = () => structuredClone(state);
export const GetState = async()=>{window.__qa.gets=(window.__qa.gets??0)+1;(window.__qa.reads??=[]).push({hidden:document.hidden,forced:window.__qa.forceHidden,stack:new Error().stack});return clone();};
export const Bootstrap = async()=>clone();
export const Refresh = async()=>clone();
export const CheckForUpdates = async()=>state.update;
export const GetDependencies = async()=>[];
export async function SaveSettings(input){window.__qa.saves.push(structuredClone(input));await new Promise(resolve=>setTimeout(resolve,window.__qa.saveDelay));if(input.upstreams.some(profile=>!profile.host))throw Error('档案地址不能为空');state.settings=structuredClone(input);return clone();}
export async function ProbeAllUpstreams(){return state.settings.upstreams.map(profile=>({...profile,reachable:true,latencyMs:55}));}
export async function SwitchUpstream(name){const profile=state.settings.upstreams.find(item=>item.name===name);Object.assign(state.settings,profile,{activeUpstream:name});return clone();}
export async function SelectFastestUpstream(){return clone();}
export async function ProbeProxy(){return state.upstream;}
export async function OpenProjectPage(){}
export async function OpenConfigFolder(){}
export async function InstallDependency(){return 'ok';}
export async function ExportReport(){return 'QA';}
export async function ExportHAR(){throw Error('模拟导出失败');}
export async function ClearRequests(){state.requests=[];return clone();}
export async function DownloadUpdate(){return state.update;}
export async function RestartToUpdate(){return state.update;}
export async function LaunchCodex(options){(window.__qa.launches??=[]).push(options);if(options.restartExisting){if(window.__qa.changePID){window.__qa.changePID=false;state.codex.mainPid=20003;return {needsRestart:true,pid:20003};}state.codex.proxyMatched=true;state.codex.proxyUrl=state.lane.interfaceUrl;return {started:true,pid:state.codex.mainPid};}return {alreadyRunning:true,pid:123};}
export async function StopLane(){state.lane.running=false;}
export async function StartLane(){state.lane.running=true;}
export async function RunTests(){state.testReport={healthy:true,score:100,results:[],durationMs:100,finishedAt:now};return state.testReport;}
export async function History(hours){window.__qa.history.push(hours);await new Promise(resolve=>setTimeout(resolve,hours===24?700:100));if(window.__qa.failHistory)throw Error('模拟历史读取失败');return [{time:hours===24?'OLD-24H':'NEW-7D',bytesDown:111,latencyMs:33}];}
`;

const browser = await chromium.launch({headless:true});
const page = await browser.newPage({viewport:{width:1280,height:800}});
const errors=[];
page.on('pageerror', error=>errors.push(error.message));
await page.route(/\/internal\/appservice\/appservice\.(js|ts)(\?.*)?$/,route=>route.fulfill({contentType:'application/javascript',body:mock}));
await page.route(/\/@wailsio_runtime\.js(\?.*)?$/,route=>route.fulfill({contentType:'application/javascript',body:'window.__qaListeners={}; export const Events={On(name,callback){window.__qaListeners[name]=callback;return ()=>delete window.__qaListeners[name];}};'}));
await page.goto(process.env.QA_URL || 'http://127.0.0.1:19245/');
await page.getByRole('button',{name:'设置',exact:true}).click();
await page.getByRole('heading',{name:'应用配置',exact:true}).waitFor();
await page.getByRole('textbox',{name:'上游地址',exact:true}).fill('127.0.0.9');
await page.getByRole('combobox',{name:'日志保留天数',exact:true}).selectOption('30');
await page.waitForFunction(()=>window.__qa.state.settings.retentionDays===30);
assert.equal(await page.evaluate(()=>window.__qa.state.settings.host),'127.0.0.1','automatic save must not apply a manual proxy draft');
assert.equal(await page.getByRole('textbox',{name:'上游地址',exact:true}).inputValue(),'127.0.0.9','manual draft must survive automatic saving');
await page.getByRole('button',{name:'取消更改',exact:true}).click();
await page.evaluate(()=>window.__qa.saveDelay=900);
const raceStart=await page.evaluate(()=>window.__qa.saves.length);
await page.getByRole('button',{name:'启动 Lane 时打开 Codex',exact:true}).click();
await page.waitForFunction(count=>window.__qa.saves.length>count,raceStart);
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.getByRole('button',{name:'设置',exact:true}).click();
await page.getByRole('combobox',{name:'内存日志容量',exact:true}).selectOption('5000');
await page.waitForFunction(()=>window.__qa.state.settings.maxMemoryRows===5000);
assert.equal(await page.evaluate(()=>window.__qa.state.settings.openCodex),true,'returning to settings must not revert an in-flight save');
await page.evaluate(()=>window.__qa.saveDelay=600);
const initialSaves=await page.evaluate(()=>window.__qa.saves.length);
await page.getByRole('button',{name:'开机自动启动',exact:true}).click();
await page.waitForFunction(count=>window.__qa.saves.length===count+1,initialSaves);
await page.getByRole('button',{name:'故障自动切换',exact:true}).click();
await page.waitForFunction(count=>window.__qa.saves.length===count+2,initialSaves);
await page.waitForFunction(()=>window.__qa.state.settings.autoFailover===true);
assert.equal(await page.getByRole('button',{name:'故障自动切换',exact:true}).evaluate(element=>element.classList.contains('off')),false);
assert.equal(await page.getByRole('button',{name:'开机自动启动',exact:true}).evaluate(element=>element.classList.contains('off')),false);
await page.evaluate(()=>{
 document.querySelector('button[aria-label="开机自动启动"]').click();
});
await page.evaluate(()=>new Promise(requestAnimationFrame));
await page.evaluate(()=>{
 Array.from(document.querySelectorAll('button')).find(element=>element.textContent.trim()==='取消更改').click();
});
const previous=await page.evaluate(()=>window.__qa.saves.length);
await page.waitForTimeout(400);
assert.equal(await page.evaluate(()=>window.__qa.saves.length),previous,'cancel must discard pending autosave');
await page.getByRole('button',{name:'开机自动启动',exact:true}).click();
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.waitForFunction(()=>window.__qa.state.settings.launchAtLogin===false);
await page.getByRole('button',{name:'设置',exact:true}).click();
const beforeEdit=await page.evaluate(()=>window.__qa.saves.length);
await page.getByRole('button',{name:'开机自动启动',exact:true}).click();
await page.waitForFunction(count=>window.__qa.saves.length>count,beforeEdit);
await page.getByRole('textbox',{name:'上游地址',exact:true}).fill('127.0.0.2');
await page.waitForFunction(()=>window.__qa.state.settings.launchAtLogin===true);
await page.waitForTimeout(100);
assert.equal(await page.getByRole('textbox',{name:'上游地址',exact:true}).inputValue(),'127.0.0.2','saving must preserve later manual edits');
assert.equal(await page.getByRole('button',{name:'保存并应用',exact:true}).isEnabled(),true);
await page.getByRole('button',{name:'保存并应用',exact:true}).click();
await page.waitForFunction(()=>window.__qa.state.settings.host==='127.0.0.2');
await page.getByRole('textbox',{name:'新档案名称',exact:true}).fill('QA profile');
await page.getByRole('textbox',{name:'新档案地址',exact:true}).fill('127.0.0.1');
await page.locator('.grid-profiles.entry-foot').getByRole('button',{name:'添加',exact:true}).click();
await page.getByRole('textbox',{name:'档案地址',exact:true}).fill('');
await page.getByRole('button',{name:'保存并应用',exact:true}).click();
await page.waitForFunction(()=>window.__qa.saves.at(-1).upstreams[0]?.host==='');
await page.getByText('配置保存失败',{exact:true}).waitFor();
assert.equal(await page.getByRole('textbox',{name:'档案名称',exact:true}).inputValue(),'QA profile','invalid profile must not disappear');
await page.getByRole('button',{name:'取消更改',exact:true}).click();
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.evaluate(()=>{
 window.__qa.beforeProfileRace=structuredClone(window.__qa.state.settings);
 Object.assign(window.__qa.state.settings,{activeUpstream:'档案 A',protocol:'http',host:'127.0.0.1',port:7897,upstreams:[{name:'档案 A',protocol:'http',host:'127.0.0.1',port:7897},{name:'档案 B',protocol:'socks5',host:'127.0.0.2',port:1080}]});
});
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'设置',exact:true}).click();
await page.getByRole('textbox',{name:'上游地址',exact:true}).fill('127.0.0.3');
await page.evaluate(()=>Object.assign(window.__qa.state.settings,{activeUpstream:'档案 B',protocol:'socks5',host:'127.0.0.2',port:1080}));
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'保存并应用',exact:true}).click();
await page.waitForFunction(()=>window.__qa.state.settings.host==='127.0.0.3');
assert.equal(await page.evaluate(()=>window.__qa.state.settings.activeUpstream),'档案 A','manual proxy draft must keep its selected profile after automatic failover');
assert.equal(await page.evaluate(()=>window.__qa.state.settings.upstreams[1].host),'127.0.0.2','saving profile A must not change profile B');
await page.getByRole('textbox',{name:'档案名称',exact:true}).first().fill('档案 A 新版');
await page.evaluate(()=>Object.assign(window.__qa.state.settings,{activeUpstream:'档案 B',protocol:'socks5',host:'127.0.0.2',port:1080}));
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'保存并应用',exact:true}).click();
await page.waitForFunction(()=>window.__qa.state.settings.activeUpstream==='档案 A 新版');
assert.equal(await page.evaluate(()=>window.__qa.state.settings.protocol),'http','renamed profile must keep its own proxy protocol');
assert.equal(await page.evaluate(()=>window.__qa.state.settings.port),7897,'renamed profile must keep its own port');
await page.evaluate(()=>window.__qa.state.settings=window.__qa.beforeProfileRace);
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.getByRole('button',{name:'24 小时',exact:true}).click();
await page.getByRole('button',{name:'7 天',exact:true}).click();
await page.getByText('NEW-7D',{exact:true}).first().waitFor();
await page.waitForTimeout(800);
assert.equal(await page.getByText('NEW-7D',{exact:true}).count()>0,true,'stale history response must be ignored');
await page.evaluate(()=>window.__qa.failHistory=true);
await page.getByRole('button',{name:'24 小时',exact:true}).click();
await page.getByRole('alert').filter({hasText:'模拟历史读取失败'}).waitFor();
assert.equal(await page.locator('.chart-svg path').count(),0,'failed history must not display live data under a historical label');
await page.evaluate(()=>window.__qa.failHistory=false);
await page.getByRole('button',{name:'重试历史',exact:true}).click();
await page.getByText('OLD-24H',{exact:true}).first().waitFor();
await page.getByRole('button',{name:'请求日志',exact:true}).click();
await page.getByRole('button',{name:'异常 1',exact:true}).click();
assert.equal(await page.getByText('pending.test',{exact:true}).count(),0,'pending connections must not count as abnormal');
await page.getByRole('button',{name:'导出 HAR',exact:true}).click();
await page.getByRole('alert').filter({hasText:'模拟导出失败'}).waitFor();
await page.evaluate(()=>{
 window.__qa.state.settings.upstreams=[{name:'主代理',protocol:'http',host:'127.0.0.1',port:7897},{name:'备用代理',protocol:'socks5',host:'127.0.0.1',port:1080}];
 window.__qa.state.codex.running=true;
 window.__qa.state.codex.proxyMatched=false;
});
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.getByRole('combobox',{name:'切换代理档案',exact:true}).selectOption('备用代理');
await page.waitForFunction(()=>window.__qa.state.settings.activeUpstream==='备用代理');
assert.equal(await page.evaluate(()=>window.__qa.state.lane.interfacePort),53999,'quick switch must keep local port');
await page.getByRole('button',{name:'接入 Codex',exact:true}).click();
await page.getByText('Codex 已在通道中运行',{exact:true}).waitFor();
assert.equal(await page.getByRole('dialog').count(),0,'cached process status must not trigger a restart prompt before backend verification');
await page.evaluate(()=>{
 window.__qa.state.settings.promptCodexProxy=true;
 Object.assign(window.__qa.state.codex,{running:true,proxyMatched:false,mainPid:20001,proxyUrl:''});
});
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('dialog').waitFor();
assert.equal(await page.getByText('重启后，Codex Lane 才会接管 Codex 并通过代理访问网络。接入成功后，可按需关闭 TUN 和系统代理。',{exact:true}).count(),1);
await page.screenshot({path:outputDir+'/restart-dialog.png'});
const launchCount=await page.evaluate(()=>window.__qa.launches.length);
await page.getByRole('button',{name:'稍后重启',exact:true}).click();
await page.evaluate(()=>window.__qaListeners['lane:codex']({data:structuredClone(window.__qa.state.codex)}));
await page.waitForTimeout(100);
assert.equal(await page.getByRole('dialog').count(),0,'later must suppress repeated prompts for the same process');
assert.equal(await page.evaluate(()=>window.__qa.launches.length),launchCount,'later must not restart Codex');
await page.evaluate(()=>{
 window.__qa.state.codex.mainPid=20002;
 window.__qa.changePID=true;
 window.__qaListeners['lane:codex']({data:structuredClone(window.__qa.state.codex)});
});
await page.getByRole('dialog').waitFor();
await page.getByRole('button',{name:'现在重启',exact:true}).click();
await page.getByRole('dialog').filter({hasText:'PID 20003'}).waitFor();
assert.equal(await page.evaluate(()=>window.__qa.launches.at(-1).expectedPid),20002);
await page.getByRole('button',{name:'现在重启',exact:true}).click();
await page.waitForFunction(()=>window.__qa.state.codex.proxyMatched===true);
await page.waitForTimeout(100);
assert.equal(await page.getByRole('dialog').count(),0,'connected process must not be prompted again');
assert.equal(await page.evaluate(()=>window.__qa.launches.at(-1).expectedPid),20003);
await page.getByRole('button',{name:'请求日志',exact:true}).click();
await page.evaluate(()=>{
 for(let index=0;index<30;index++) {
  const request={...window.__qa.state.requests[0],id:'batch-'+index,host:'batch-'+index+'.test',status:200,result:'ok'};
  window.__qa.state.requests.unshift({...request,bytesDown:2});
  for(let update=0;update<3;update++) window.__qaListeners['lane:request']({data:{event:'request:update',request:{...request,bytesDown:update}}});
 }
});
await page.getByText('batch-29.test',{exact:true}).first().waitFor();
assert.equal(await page.locator('.request-table-wrap tbody tr').count(),32,'batched request updates must not duplicate rows');
const beforeHidden=await page.evaluate(()=>{
 window.__qa.forceHidden=true;
 Object.defineProperty(document,'hidden',{configurable:true,get:()=>window.__qa.forceHidden});
 document.dispatchEvent(new Event('visibilitychange'));
 window.__qaListeners['lane:request']({data:{event:'request:finished',request:{id:'hidden',host:'hidden.test',status:200,result:'ok'}}});
 return window.__qa.gets;
});
await page.waitForTimeout(600);
assert.equal(await page.evaluate(()=>window.__qa.gets),beforeHidden,'hidden request events must not schedule snapshot refreshes: '+JSON.stringify(await page.evaluate(()=>window.__qa.reads)));
assert.equal(await page.getByText('hidden.test',{exact:true}).count(),0);
await page.evaluate(()=>{
 window.__qa.forceHidden=false;
 window.__qa.state.requests=[{...window.__qa.state.requests[0],host:'restored.test'}];
 document.dispatchEvent(new Event('visibilitychange'));
});
await page.getByText('restored.test',{exact:true}).first().waitFor();
await page.getByRole('button',{name:'暂停',exact:true}).click();
await page.getByRole('button',{name:'清空',exact:true}).click();
await page.getByText('等待第一笔请求',{exact:true}).waitFor();
assert.equal(await page.locator('.request-table-wrap tbody tr').count(),0,'clearing paused logs must clear visible rows');
await page.evaluate(()=>{
 window.__qa.state.requests=[{id:'direct',time:'13:00:00.123',host:'direct.test',method:'CONNECT',source:'interface',port:443,scheme:'https',status:200,result:'ok',upstream:'DIRECT'}];
});
await page.getByRole('button',{name:'继续',exact:true}).click();
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByText('TLS 隧道 · 按规则直连',{exact:true}).waitFor();
await page.getByRole('button',{name:'设置',exact:true}).click();
for(const [label,key] of [['启动 Lane 时打开 Codex','openCodex'],['退出 Lane 时同步停止 Codex','stopCodexOnExit'],['关闭窗口时最小化到托盘','minimizeToTray']]) {
 const before=await page.evaluate(key=>window.__qa.state.settings[key],key);
 const control=page.getByRole('button',{name:label,exact:true});
 await control.click();
 await page.waitForFunction(({key,before})=>window.__qa.state.settings[key]!==before,{key,before});
 assert.equal(await control.getAttribute('aria-pressed'),String(!before));
}
await page.getByRole('combobox',{name:'日志保留天数',exact:true}).selectOption('30');
await page.waitForFunction(()=>window.__qa.state.settings.retentionDays===30);
await page.getByRole('combobox',{name:'内存日志容量',exact:true}).selectOption('5000');
await page.waitForFunction(()=>window.__qa.state.settings.maxMemoryRows===5000);
await page.getByRole('button',{name:'恢复默认',exact:true}).click();
assert.equal(await page.getByRole('combobox',{name:'日志保留天数',exact:true}).inputValue(),'14');
await page.getByRole('button',{name:'取消更改',exact:true}).click();
assert.equal(await page.getByRole('combobox',{name:'日志保留天数',exact:true}).inputValue(),'30');
await page.evaluate(()=>{
 window.__qa.state.update={status:'ready',currentVersion:'0.1.42',latestVersion:'99.0.0',available:true,downloaded:100,total:100};
});
await page.getByRole('button',{name:'刷新状态',exact:true}).click();
await page.getByRole('button',{name:'重启并更新',exact:true}).click();
await page.getByRole('dialog').waitFor();
assert.equal(await page.evaluate(()=>document.activeElement?.textContent),'取消');
await page.keyboard.press('Shift+Tab');
assert.equal(await page.evaluate(()=>document.activeElement?.textContent.trim()),'重启并更新');
await page.keyboard.press('Escape');
assert.equal(await page.getByRole('dialog').count(),0);
await page.getByRole('button',{name:'总览',exact:true}).click();
await page.getByRole('button',{name:'30 分钟',exact:true}).click();
const chart=page.getByRole('slider',{name:'流量与延迟图表，移动指针或按左右方向键查看读数',exact:true});
await chart.focus();
await page.keyboard.press('ArrowLeft');
await page.locator('.chart-tip').waitFor();
await page.keyboard.press('Escape');
assert.equal(await page.locator('.chart-tip').count(),0);
await page.waitForTimeout(4800);
await mkdir(outputDir,{recursive:true});
for(const [width,height] of [[1360,860],[1120,720],[900,600]]) {
 await page.setViewportSize({width,height});
 for(const label of ['总览','请求日志','检查','设置']) {
  await page.getByRole('button',{name:label,exact:true}).click();
  await page.waitForTimeout(120);
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
  assert.equal(overflow,false, label+' must fit viewport '+width);
  const nested=await page.evaluate(()=>Array.from(document.querySelectorAll('.view.active *')).filter(element=>getComputedStyle(element).overflowY==='auto' && element.scrollHeight>element.clientHeight+1).map(element=>element.className));
  assert.deepEqual(nested,[],label+' must use only the page vertical scrollbar at '+width);
  if(label==='设置') {
   for(const chip of await page.locator('.endpoint-address').all()) {
    assert.equal(await chip.evaluate(element=>element.scrollWidth<=element.clientWidth),true,'local endpoint address must fit its border at '+width);
   }
  }
  await page.screenshot({path:outputDir+'/'+width+'-'+label+'.png'});
 }
}
assert.deepEqual(errors,[]);
console.log('PASS: settings, proxy switching, log operations, restart confirmation, update dialog focus, chart keyboard readouts; four pages at 1360x860 / 1120x720 / 900x600 with one vertical scrollbar and no address overflow.');
await browser.close();
