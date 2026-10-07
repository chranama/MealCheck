#!/usr/bin/env python3
"""Run a scoped connectivity outage against the temporary lab LaunchAgent.

Requires the lab deployment already Ready. Changes only the dedicated lab agent
endpoint; restores its original direct endpoint even if verification fails.
"""
import datetime
import json
import os
from pathlib import Path
import signal
import subprocess
import time

root = Path(__file__).resolve().parents[2]
lab = Path(os.environ.get('MEALCHECK_LAB_ROOT', '/tmp/controller-lab'))
cli = [str(lab / 'bin/mealcheck-controller'), 'get', '--state-dir', str(lab / 'controller-state')]
endpoint = os.environ.get('MEALCHECK_CONTROLLER_ENGINE', 'unix://' + os.path.expanduser('~/.docker/run/docker.sock'))
if not endpoint.startswith('unix://'):
    raise SystemExit('local Unix engine required')
proxy_dir = lab / 'proxy'
proxy_dir.mkdir(mode=0o700, exist_ok=True)
os.chmod(proxy_dir, 0o700)
listen = str(proxy_dir / 'engine.sock')
env = dict(os.environ, MEALCHECK_LAUNCHAGENT_PATH=str(lab / 'dev.mealcheck.controller.lab.plist'))

def status():
    return json.loads(subprocess.check_output(cli, stderr=subprocess.DEVNULL))

def identities(value):
    return {r: v['id'] for r, v in value['status']['resources'].items()}

def wait_phase(phase, newer_than, timeout=60):
    start = time.monotonic()
    while time.monotonic() - start < timeout:
        try:
            value = status()
            observed = value['status'].get('lastObservation', '')
            # Unknown observations retain last-successful-observation timestamp.
            if value['status']['phase'] == phase and (phase != 'Ready' or observed > newer_than):
                return value, round(time.monotonic() - start, 3)
        except (subprocess.CalledProcessError, KeyError):
            pass
        time.sleep(.2)
    raise RuntimeError('timed out waiting for ' + phase)

def install(engine):
    subprocess.run([str(root/'deploy/controller/uninstall-launchagent.sh')], env=env, check=True)
    subprocess.run([str(root/'deploy/controller/install-launchagent.sh')], env=dict(env, MEALCHECK_CONTROLLER_ENGINE=engine), check=True)

before = status()
if before['document'].get('deploymentID') != 'mealcheck-lab':
    raise SystemExit('refusing non-lab deployment')
if before['status']['phase'] != 'Ready':
    raise SystemExit('lab must be Ready before outage demonstration')
proxy = subprocess.Popen(['python3', str(root/'deploy/controller/engine-outage.py'), '--listen', listen, '--target', endpoint[7:]])
try:
    for _ in range(100):
        if os.path.exists(listen):
            break
        if proxy.poll() is not None:
            raise RuntimeError('proxy exited before binding')
        time.sleep(.05)
    install('unix://' + listen)
    ready, _ = wait_phase('Ready', before['status']['lastObservation'])
    assert identities(ready) == identities(before)
    os.kill(proxy.pid, signal.SIGUSR1)
    degraded, detection = wait_phase('Degraded', '')
    assert identities(degraded) == identities(before), 'outage changed bindings'
    # Direct engine remains available, so this is caller connectivity, not shutdown.
    subprocess.run(['docker', '--host', endpoint, 'info', '--format', '{{.ServerVersion}}'], check=True)
    os.kill(proxy.pid, signal.SIGUSR2)
    recovered, recovery = wait_phase('Ready', degraded['status']['lastObservation'])
    assert identities(recovered) == identities(before), 'recovery replaced resources'
    assert recovered['generation'] == before['generation']
    evidence = {'testedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'scope': 'controller-to-engine connectivity only; shared engine remained running', 'detectionSeconds': detection, 'recoverySeconds': recovery, 'outagePhase': degraded['status']['phase'], 'outageConditions': degraded['status']['conditions'], 'recoveredPhase': recovered['status']['phase'], 'resourceIDsUnchanged': True, 'generation': recovered['generation']}
    (lab/'engine-outage-evidence.json').write_text(json.dumps(evidence, indent=2))
    print(json.dumps(evidence, indent=2))
finally:
    try:
        install(endpoint)
    finally:
        proxy.terminate()
        proxy.wait(timeout=10)
