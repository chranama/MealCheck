#!/usr/bin/env python3
"""Render a deployment manifest referencing a separately trusted system definition."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--lab-root', default=os.environ.get('MEALCHECK_LAB_ROOT', '/tmp/controller-lab'))
parser.add_argument('--system-file', required=True, help='engineer-approved catalog JSON; never copied or changed by deployment input')
parser.add_argument('--controller-binary', required=True, help='built controller used for canonical system digest')
parser.add_argument('--deployment-id', default='mealcheck-lab')
parser.add_argument('--api-host-port', type=int, default=18080)
parser.add_argument('--desired-state', choices=['Running', 'Stopped'], default='Running')
parser.add_argument('--output', required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
pins = dict(line.split('=', 1) for line in (root/'deploy/controller/images.env').read_text().splitlines() if line and not line.startswith('#'))
lab = Path(args.lab_root).resolve(strict=True)
model = (lab/'models/model.gguf').resolve(strict=True)
if not model.is_relative_to((lab/'models').resolve(strict=True)):
    parser.error('model must remain within lab model root')
hash_value = hashlib.sha256()
with model.open('rb') as stream:
    for chunk in iter(lambda: stream.read(1 << 20), b''):
        hash_value.update(chunk)
if hash_value.hexdigest() != pins['MODEL_SHA256']:
    parser.error('model checksum does not match approved profile')
system_file = Path(args.system_file).resolve(strict=True)
system = json.loads(system_file.read_text())
digest = subprocess.check_output([args.controller_binary, 'system-digest', '--file', str(system_file)], text=True).strip()
if not re.fullmatch(r'sha256:[0-9a-f]{64}', digest):
    parser.error('controller returned invalid system digest')
document = {'apiVersion': 'mealcheck.dev/deployment/v1alpha1', 'deploymentID': args.deployment_id, 'desiredState': args.desired_state, 'system': {'name': system['name'], 'version': system['version'], 'digest': digest}, 'parameters': {'modelPath': str(model), 'secretProfile': 'mealcheck-lab', 'apiHostPort': args.api_host_port}}
# Exclusive creation avoids replacing previously reviewed desired state. No secret contents enter JSON.
with Path(args.output).open('x') as stream:
    json.dump(document, stream, indent=2)
    stream.write('\n')
