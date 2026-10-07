#!/usr/bin/env python3
"""Explicit isolated-lab drift injection; requires a Ready controller workload."""
import argparse,datetime,hashlib,json,subprocess,time,urllib.request
p=argparse.ArgumentParser();p.add_argument('--engine',required=True);p.add_argument('--binary',required=True);p.add_argument('--state-dir',required=True);p.add_argument('--report-run',required=True);p.add_argument('--output',required=True);a=p.parse_args()
def cli(command):return json.loads(subprocess.check_output([a.binary,command,'--state-dir',a.state_dir]))
def report(s):
 port=s['document']['spec']['apiHostPort']
 return hashlib.sha256(urllib.request.urlopen(f'http://127.0.0.1:{port}/api/runs/{a.report_run}/artifacts/report.json',timeout=5).read()).hexdigest()
def engine(*args):return subprocess.check_output(['docker','--host',a.engine,*args],text=True,stderr=subprocess.DEVNULL).strip()
def observation_time(s):return datetime.datetime.fromisoformat(s['status']['lastObservation'].replace('Z','+00:00')).timestamp()
if not a.engine.startswith('unix:///'): raise SystemExit('explicit local Unix engine required')
initial=cli('get');assert initial['status']['phase']=='Ready'
assert initial['document']['deploymentID']=='mealcheck-lab' and initial['document']['spec']['apiHostPort']==18080, 'isolated lab required'
network_binding=initial['status']['resources']['network']
net=json.loads(engine('network','inspect',network_binding['name']))[0]
assert net['Id']==network_binding['id']
label_prefix='dev.mealcheck.controller.'
owner=net['Labels'][label_prefix+'owner']
keys=['profile','apiImage','postgresImage','modelImage','modelPath','secretProfile','apiHostPort','dataPolicy']
fingerprint=hashlib.sha256(json.dumps({k:initial['document']['spec'][k] for k in keys},separators=(',',':')).encode()).hexdigest()
assert net['Labels'][label_prefix+'fingerprint']==fingerprint and net['Labels'][label_prefix+'deployment']=='mealcheck-lab'
baseline=report(initial);results=[]
for role,action in [('api','kill'),('model','kill'),('api','remove')]:
 s=cli('get');binding=s['status']['resources'][role];name=binding['name'];old_id=binding['id'];start=time.monotonic();wall=time.time()
 actual=json.loads(engine('inspect',name))[0]
 assert actual['Id']==old_id
 for key,value in {'owner':owner,'deployment':'mealcheck-lab','role':role,'fingerprint':fingerprint}.items():assert actual['Config']['Labels'].get(label_prefix+key)==value, 'ownership mismatch'
 if role=='api':
  ports=actual['HostConfig']['PortBindings'];assert ports=={'8080/tcp':[{'HostIp':'127.0.0.1','HostPort':'18080'}]}, 'private ingress required'
 if action=='kill':engine('kill',old_id)
 else:engine('rm','-f',old_id)
 detected=acted=None
 while time.monotonic()-start<150:
  s=cli('get');elapsed=round(time.monotonic()-start,3)
  if observation_time(s)>wall and detected is None:detected=elapsed
  try:
   observed=json.loads(engine('inspect',name))[0]
   if observed['State']['Running'] and datetime.datetime.fromisoformat(observed['State']['StartedAt'].replace('Z','+00:00')).timestamp()>wall and acted is None:acted=elapsed
  except subprocess.CalledProcessError:pass
  if s['status']['phase']=='Blocked':raise RuntimeError(s['status'])
  if s['status']['phase']=='Ready' and observation_time(s)>wall and acted is not None:break
  time.sleep(.25)
 else:raise RuntimeError('recovery timeout')
 current=report(s);assert current==baseline
 new_id=s['status']['resources'][role]['id'];assert (new_id==old_id)==(action=='kill')
 results.append({'role':role,'fault':action,'detectionSeconds':detected,'runningSeconds':acted,'readySeconds':elapsed,'reportRetained':True,'sameContainerID':new_id==old_id})
 print(json.dumps(results[-1]),flush=True)
json.dump({'scenarios':results,'reportSHA256':baseline},open(a.output,'w'),indent=2)
