#!/usr/bin/env python3
"""Render a nonsecret lab desired-state document from explicit pinned images."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--lab-root', default=os.environ.get('MEALCHECK_LAB_ROOT', '/tmp/controller-lab'))
parser.add_argument('--api-image', required=True, help='preloaded registry/name@sha256:digest')
parser.add_argument('--output', required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
pins = {}
for line in (root/'deploy/controller/images.env').read_text().splitlines():
    if line and not line.startswith('#'):
        key, value = line.split('=', 1)
        pins[key] = value
images = {'apiImage': args.api_image, 'postgresImage': pins['POSTGRES_IMAGE'], 'modelImage': pins['MODEL_IMAGE']}
for name, value in images.items():
    if not re.fullmatch(r'[^\s]+@sha256:[0-9a-f]{64}', value):
        parser.error(name + ' must be digest pinned')
lab = Path(args.lab_root).resolve(strict=True)
model = (lab/'models/model.gguf').resolve(strict=True)
if not model.is_relative_to(lab/'models'):
    parser.error('model must remain within lab model root')
digest = hashlib.sha256()
with model.open('rb') as stream:
    for chunk in iter(lambda: stream.read(1 << 20), b''):
        digest.update(chunk)
if digest.hexdigest() != pins['MODEL_SHA256']:
    parser.error('model checksum does not match approved profile')
document = {'apiVersion': 'mealcheck.dev/v1alpha1', 'deploymentID': 'mealcheck-lab', 'desiredState': 'Running', 'spec': {'profile': 'cpu-local-model-v1', **images, 'modelPath': str(model), 'secretProfile': 'mealcheck-lab', 'apiHostPort': 18080, 'dataPolicy': 'Retain'}}
output = Path(args.output)
# No credentials enter this document. Refuse accidental replacement.
with output.open('x') as stream:
    json.dump(document, stream, indent=2)
    stream.write('\n')
