#!/usr/bin/env python3
"""Explicit isolated-lab drift injection; requires a Ready controller workload."""
import argparse,datetime,hashlib,json,subprocess,time,urllib.request
p=argparse.ArgumentParser();p.add_argument('--binary',required=True);p.add_argument('--state-dir',required=True);p.add_argument('--report-run',required=True);p.add_argument('--output',required=True);a=p.parse_args()
def cli(command):return json.loads(subprocess.check_output([a.binary,command,'--state-dir',a.state_dir]))
def report(s):
 port=s['document']['spec']['apiHostPort']
 return hashlib.sha256(urllib.request.urlopen(f'http://127.0.0.1:{port}/api/runs/{a.report_run}/artifacts/report.json',timeout=5).read()).hexdigest()
def engine(*args):return subprocess.check_output(['docker',*args],text=True,stderr=subprocess.DEVNULL).strip()
def observation_time(s):return datetime.datetime.fromisoformat(s['status']['lastObservation'].replace('Z','+00:00')).timestamp()
initial=cli('get');assert initial['status']['phase']=='Ready';baseline=report(initial);results=[]
for role,action in [('api','kill'),('model','kill'),('api','remove')]:
 s=cli('get');binding=s['status']['resources'][role];name=binding['name'];old_id=binding['id'];start=time.monotonic();wall=time.time()
 if action=='kill':engine('kill',name)
 else:engine('rm','-f',name)
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
