# M5 recovery evidence — 2026-10-07

Real ARM64 Docker Desktop workload, native controller supervised by a dedicated
user LaunchAgent. Five-second observation polling. Synthetic completed report
hash remained `77b2e528d71fe0c670c89e22f376258d43e2b7d5833bcd294bcc8f9e420093f7`.

| Injected fault | Fresh detection | Running observed | Ready observed | Outcome |
| --- | --- | --- | --- | --- |
| API SIGKILL | 2.239s | 2.239s | 7.160s | Same container ID, report retained |
| Model SIGKILL | 5.045s | 5.045s | 9.873s | Same container ID, report retained |
| API force removal | 4.959s | 10.191s | 15.151s | New API container ID, same retained report |

Measurements are sampled by the test harness: detection is the first fresh
controller observation, running is an Engine observation, and Ready is current
controller status. They are distinct from end-to-end model inference. The full
real workflow was repeated after these recovery scenarios; its result is
recorded in the final milestone evidence.

Reproduce against an explicitly prepared isolated Ready lab:

```sh
python3 deploy/controller/test-recovery.py \
  --binary /tmp/controller-lab/bin/mealcheck-controller \
  --state-dir /tmp/controller-lab/controller-state \
  --report-run <completed-synthetic-run-id> --output /tmp/recovery-results.json
```

This harness intentionally kills/removes only the API/model named in the accepted
lab state. It performs no volume purge. Keep other work out of this isolated lab
while injecting failures.

Deterministic fake-provider tests establish persisted five-start/ten-minute
budgets (including initial start),120s startup timeout, retry across restart,
1–30s jittered backoff, missing/replaced data blocking, crash after mutation before
commit, deletion versus in-flight mutation, cancellation and classified errors.
These scenarios use controllable clocks rather than elapsed production waits;
they do not claim real Docker crash-loop or missing-data destruction experiments.
See [failure policy](failure-policy.md) for named tests and exact reset semantics.

Incarnation/ownership labels defend against accidental collisions/replacements
within the controller ownership contract. A privileged Docker administrator can
copy or alter all labels and is outside that trust boundary.
